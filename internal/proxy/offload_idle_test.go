package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/schedulingcost"
)

func handlerBlockingOnlyInferencePath(t *testing.T, gate chan struct{}, inferencePath string, body string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != inferencePath {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}],"model_name":"ready"}`))
			return
		}
		<-gate
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
}

func TestIdleForBorrowedWorkTrueWhenNothingRunning(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.NotFoundHandler(), http.NotFoundHandler(), map[string]string{
		"text-only": `{"model_param":"C:\\models\\llm.gguf"}`,
	})
	if !service.scheduler.idleForBorrowedWork() {
		t.Fatal("a node with nothing running reported busy")
	}
}

func TestIdleForBorrowedWorkFalseWhileATextRequestRuns(t *testing.T) {
	gate := make(chan struct{})
	service, _, _ := newSplitTestServiceWithConfigContents(t,
		handlerBlockingOnlyInferencePath(t, gate, "/v1/chat/completions", `{"model":"backend","choices":[{"message":{"content":"ok"}}]}`),
		http.NotFoundHandler(),
		map[string]string{
			"text-only": `{"model_param":"C:\\models\\llm.gguf"}`,
		},
	)

	done := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"text-only","messages":[]}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		done <- recorder.Code
	}()

	waitForCondition(t, func() bool { return !service.scheduler.idleForBorrowedWork() })

	close(gate)
	if code := <-done; code != http.StatusOK {
		t.Fatalf("text request status %d, want 200", code)
	}
	waitForCondition(t, func() bool { return service.scheduler.idleForBorrowedWork() })
}

func TestIdleForBorrowedWorkFalseWhileAnImageRequestRuns(t *testing.T) {
	gate := make(chan struct{})
	service, _, _ := newSplitTestServiceWithConfigContents(t,
		http.NotFoundHandler(),
		handlerBlockingOnlyInferencePath(t, gate, "/v1/images/generations", `{"model":"backend","data":[]}`),
		map[string]string{
			"image-only": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
		},
	)

	done := make(chan int, 1)
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-only-dream","prompt":"cat"}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		done <- recorder.Code
	}()

	waitForCondition(t, func() bool { return !service.scheduler.idleForBorrowedWork() })

	close(gate)
	<-done
	waitForCondition(t, func() bool { return service.scheduler.idleForBorrowedWork() })
}

func TestBorrowedWorkAloneKeepsTheNodeIdle(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.NotFoundHandler(), http.NotFoundHandler(), map[string]string{
		"text-only": `{"model_param":"C:\\models\\llm.gguf"}`,
	})
	entry := service.scheduler.textQueue.Enqueue(queuedRequest{modelID: "group", work: schedulingcost.TextWork(100, 20), requiredContext: 2048, origin: borrowedFromPeer}, nodeActivity(true), time.Now())
	if outcome, ok := outcomeNow(t, entry); !ok || outcome != offloadAdmitted {
		t.Fatalf("borrowed entry outcome = %v ok=%t, want admitted on an idle node", outcome, ok)
	}
	if !service.scheduler.idleForBorrowedWork() {
		t.Fatal("an admitted borrowed entry alone made the node report busy")
	}
}

func TestSeparatePoolRuntimeDoesNotMakeTheNodeBusy(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.NotFoundHandler(), http.NotFoundHandler(), map[string]string{
		"text-only": `{"model_param":"C:\\models\\llm.gguf"}`,
	})
	if service.separatePool == nil {
		t.Skip("separate runtime pool not constructed for this fixture")
	}
	if !service.scheduler.idleForBorrowedWork() {
		t.Fatal("an idle main line reported busy before any separate-pool activity was introduced")
	}
}

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was never satisfied")
}

func TestOnlyTheLastReleaseStampsWhenTheRuntimeWentIdle(t *testing.T) {
	state := newActiveConfigState()
	state.users = 2
	releaseFirst, releaseSecond := releaseActiveConfigOnce(state), releaseActiveConfigOnce(state)

	releaseFirst()
	if !state.ownIdleSince.IsZero() {
		t.Fatal("idle was stamped while a user still held the runtime")
	}
	beforeLastRelease := time.Now()
	releaseSecond()
	if state.ownIdleSince.Before(beforeLastRelease) {
		t.Fatalf("idle since %v, want the moment the last user released", state.ownIdleSince)
	}
}

func TestIdleForIsZeroWhileARequestRunsAndCountsUpAfterwards(t *testing.T) {
	gate := make(chan struct{})
	service, _, _ := newSplitTestServiceWithConfigContents(t,
		handlerBlockingOnlyInferencePath(t, gate, "/v1/chat/completions", `{"model":"backend","choices":[{"message":{"content":"ok"}}]}`),
		http.NotFoundHandler(),
		map[string]string{"text-only": `{"model_param":"C:\\models\\llm.gguf"}`},
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"text-only","messages":[]}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(httptest.NewRecorder(), request)
	}()
	waitForCondition(t, func() bool { return service.requestsRunningOnEveryBackendFamily() > 0 })

	if idle := service.scheduler.idleFor(time.Now()); idle != 0 {
		t.Fatalf("idle for %v while a request runs, want 0", idle)
	}
	close(gate)
	<-done
	if idle := service.scheduler.idleFor(time.Now().Add(6 * time.Second)); idle < 6*time.Second {
		t.Fatalf("idle for %v six seconds after the request finished, want at least 6s", idle)
	}
}

func TestHelperIdleClockIgnoresBorrowedWork(t *testing.T) {
	service := newBorrowRestoreTestService(t)
	mode := service.currentBackendMode()
	if err := service.loadLocalConfig(context.Background(), mode, "a", "a.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}
	ownWorkEnded, idle := service.ownWorkIdleSince()
	if !idle {
		t.Fatal("helper busy after its own load finished")
	}

	if err := service.loadLocalConfig(borrowedContext(), mode, "b", "b.kcpps", readinessText); err != nil {
		t.Fatal(err)
	}

	if idleSince, idle := service.ownWorkIdleSince(); !idle || !idleSince.Equal(ownWorkEnded) {
		t.Fatalf("idle since %v idle=%t, want still idle since its own work ended at %v: borrowed work restarted the probe clock", idleSince, idle, ownWorkEnded)
	}
}

func TestHelperIsBusyWhileLoadingItsOwnModel(t *testing.T) {
	service, gate := serviceWithBlockingReload(t)

	loaded := loadInBackground(service, context.Background())

	waitForCondition(t, func() bool {
		_, idle := service.ownWorkIdleSince()
		return !idle
	})
	close(gate)
	if err := <-loaded; err != nil {
		t.Fatal(err)
	}
	if _, idle := service.ownWorkIdleSince(); !idle {
		t.Fatal("helper still busy after its own load finished")
	}
}

func TestBorrowedLoadLeavesTheHelperIdle(t *testing.T) {
	service, gate := serviceWithBlockingReload(t)
	runtime := defaultFamilyRuntime(t, service, readinessText)

	loaded := loadInBackground(service, borrowedContext())

	waitForCondition(t, func() bool {
		runtime.state.mu.Lock()
		defer runtime.state.mu.Unlock()
		return runtime.state.switching
	})
	if _, idle := service.ownWorkIdleSince(); !idle {
		t.Fatal("a load for borrowed work made the helper look busy with its own")
	}
	close(gate)
	if err := <-loaded; err != nil {
		t.Fatal(err)
	}
}

func serviceWithBlockingReload(t *testing.T) (*Service, chan struct{}) {
	t.Helper()
	gate := make(chan struct{})
	service, backend := newTestServiceWithModels(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}), "a")
	backend.onReload = func(string) { <-gate }
	return service, gate
}

func loadInBackground(service *Service, ctx context.Context) <-chan error {
	loaded := make(chan error, 1)
	go func() {
		loaded <- service.loadLocalConfig(ctx, service.currentBackendMode(), "a", "a.kcpps", readinessText)
	}()
	return loaded
}
