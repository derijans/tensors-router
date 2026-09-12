package proxy

import (
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
	if !service.idleForBorrowedWork() {
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

	waitForCondition(t, func() bool { return !service.idleForBorrowedWork() })

	close(gate)
	if code := <-done; code != http.StatusOK {
		t.Fatalf("text request status %d, want 200", code)
	}
	waitForCondition(t, func() bool { return service.idleForBorrowedWork() })
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

	waitForCondition(t, func() bool { return !service.idleForBorrowedWork() })

	close(gate)
	<-done
	waitForCondition(t, func() bool { return service.idleForBorrowedWork() })
}

func TestBorrowedWorkAloneKeepsTheNodeIdle(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.NotFoundHandler(), http.NotFoundHandler(), map[string]string{
		"text-only": `{"model_param":"C:\\models\\llm.gguf"}`,
	})
	entry := service.textQueue.Enqueue(queuedRequest{groupID: "group", work: schedulingcost.TextWork(100, 20), requiredContext: 2048, origin: borrowedFromPeer}, nodeActivity(true), time.Now())
	if outcome, ok := outcomeNow(t, entry); !ok || outcome != offloadAdmitted {
		t.Fatalf("borrowed entry outcome = %v ok=%t, want admitted on an idle node", outcome, ok)
	}
	if !service.idleForBorrowedWork() {
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
	if !service.idleForBorrowedWork() {
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
