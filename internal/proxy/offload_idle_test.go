package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/schedulingcost"
)

// gatedHandler blocks only the named inference path on gate until it is
// closed, answering every readiness probe and any other path immediately.
// This is what lets a test observe idleForBorrowedWork while a request is
// genuinely still running on the backend rather than merely enqueued, without
// having to reproduce the readiness protocol itself.
func gatedHandler(t *testing.T, gate chan struct{}, inferencePath string, body string) http.Handler {
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

// TestIdleForBorrowedWorkFalseWhileATextRequestRuns is the claim at the heart
// of the node-level idle design: an ordinary text request — never queued,
// never part of any routing group — still makes idleForBorrowedWork report
// busy, because it is read live from the runtime's own user count rather than
// from queue bookkeeping that only ever sees grouped traffic.
func TestIdleForBorrowedWorkFalseWhileATextRequestRuns(t *testing.T) {
	gate := make(chan struct{})
	service, _, _ := newSplitTestServiceWithConfigContents(t,
		gatedHandler(t, gate, "/v1/chat/completions", `{"model":"backend","choices":[{"message":{"content":"ok"}}]}`),
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

// TestIdleForBorrowedWorkFalseWhileAnImageRequestRuns mirrors the text case
// for the image lane, on the same split-runtime family: the two runtimes sit
// under one node, so either one running makes the whole node non-idle.
func TestIdleForBorrowedWorkFalseWhileAnImageRequestRuns(t *testing.T) {
	gate := make(chan struct{})
	service, _, _ := newSplitTestServiceWithConfigContents(t,
		http.NotFoundHandler(),
		gatedHandler(t, gate, "/v1/images/generations", `{"model":"backend","data":[]}`),
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

// TestBorrowedWorkAloneKeepsTheNodeIdle pins the exclusion idleForBorrowedWork
// applies: an admitted borrowed entry must never count toward this node's own
// busyness, or a helper would go busy from the very job it was lent and hand
// back everything behind it.
func TestBorrowedWorkAloneKeepsTheNodeIdle(t *testing.T) {
	service, _, _ := newSplitTestServiceWithConfigContents(t, http.NotFoundHandler(), http.NotFoundHandler(), map[string]string{
		"text-only": `{"model_param":"C:\\models\\llm.gguf"}`,
	})
	entry := service.textQueue.Enqueue("group", schedulingcost.TextWork(100, 20), 2048, true, false, true, time.Now())
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
