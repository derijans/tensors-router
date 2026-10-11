package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func borrowedRequestWithPreemption(t *testing.T) (context.Context, *borrowedPreemption) {
	t.Helper()
	request := withBorrowedPreemption(markBorrowedRequest(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)))
	return request.Context(), contextBorrowedPreemption(request.Context())
}

func admittedBorrowedWith(t *testing.T, queue *offloadQueue, preemption *borrowedPreemption) *offloadEntry {
	t.Helper()
	entry := queue.Enqueue(queuedRequest{modelID: "group", origin: borrowedFromPeer, preemption: preemption}, nodeActivity(true), time.Now())
	mustBeAdmitted(t, entry, "borrowed request")
	return entry
}

func TestOwnRequestArrivingTakesBackBorrowedWorkThatHasNotStarted(t *testing.T) {
	queue := newOffloadQueue(2)
	_, preemption := borrowedRequestWithPreemption(t)
	admittedBorrowedWith(t, queue, preemption)

	enqueueNativeOnIdleNode(queue, "group", 10)

	if !preemption.wasPreempted() {
		t.Fatal("borrowed request kept its place, want it taken back for the node's own request")
	}
}

func TestOwnRequestArrivingNeverCancelsBorrowedWorkThatIsGenerating(t *testing.T) {
	queue := newOffloadQueue(2)
	_, preemption := borrowedRequestWithPreemption(t)
	admittedBorrowedWith(t, queue, preemption)
	if !preemption.claimStart() {
		t.Fatal("a request nobody preempted could not start")
	}

	enqueueNativeOnIdleNode(queue, "group", 10)

	if preemption.wasPreempted() {
		t.Fatal("generating borrowed request was cancelled")
	}
}

func TestBorrowedRequestTakenBackBeforeItStartedCannotStartLater(t *testing.T) {
	_, preemption := borrowedRequestWithPreemption(t)
	preemption.preemptUnlessStarted()

	if preemption.claimStart() {
		t.Fatal("a request already handed back to its owner was allowed to start")
	}
}

func TestPreemptedBorrowedRequestWaitingForTheRuntimeIsReturned(t *testing.T) {
	service := newBorrowRestoreTestService(t)
	mode := service.currentBackendMode()
	_, releaseOwn, _, err := service.acquireModelConfigForBackendMode(mode, context.Background(), "a", "a.kcpps", readinessText, false)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseOwn()
	ctx, preemption := borrowedRequestWithPreemption(t)
	acquired := make(chan error, 1)
	go func() {
		_, release, _, acquireErr := service.acquireModelConfigForBackendMode(mode, ctx, "b", "b.kcpps", readinessText, false)
		if acquireErr == nil {
			release()
		}
		acquired <- acquireErr
	}()
	waitForQueuedLaneRequest(t, service)

	preemption.preemptUnlessStarted()

	select {
	case acquireErr := <-acquired:
		if !errors.Is(acquireErr, errOffloadReturned) {
			t.Fatalf("acquire error = %v, want the request returned to its owner", acquireErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("preempted borrowed request is still waiting for the runtime")
	}
	if _, filename := defaultFamilyRuntime(t, service, readinessText).state.loadedModel(); filename != "a.kcpps" {
		t.Fatalf("loaded model = %q, want the node's own model untouched", filename)
	}
}

func TestPreemptionAfterBorrowedRequestClaimedTheRuntimeLeavesItRunning(t *testing.T) {
	service := newBorrowRestoreTestService(t)
	mode := service.currentBackendMode()
	ctx, preemption := borrowedRequestWithPreemption(t)
	_, release, _, err := service.acquireModelConfigForBackendMode(mode, ctx, "b", "b.kcpps", readinessText, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	preemption.preemptUnlessStarted()

	if preemption.wasPreempted() {
		t.Fatal("request that already holds the runtime was preempted")
	}
}

func TestReturnedBorrowedRequestAnswersConflict(t *testing.T) {
	status, code, _ := backendFailureResponse(errOffloadReturned)

	if status != http.StatusConflict || code != offloadReturnedCode {
		t.Fatalf("response = %d %s, want 409 %s so the owner serves the request itself", status, code, offloadReturnedCode)
	}
}

func waitForQueuedLaneRequest(t *testing.T, service *Service) {
	t.Helper()
	state := defaultFamilyRuntime(t, service, readinessText).state
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state.mu.Lock()
		queued := state.queuedWaitersLocked()
		state.mu.Unlock()
		if queued > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("request never joined the runtime queue")
}
