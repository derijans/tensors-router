package proxy

import (
	"net/http"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/siteapi"
)

func TestNodeStateListsRequestsTheRouterHoldsBackFromTheBackend(t *testing.T) {
	gate := make(chan struct{})
	service := newLinkedImageService(t, gate, true)
	done := make(chan int, 3)
	for range 3 {
		go func() { done <- postImage(service).Code }()
	}
	waitForBacklog(t, service, 3)

	held := service.localNodeState().HeldRequests

	close(gate)
	for range 3 {
		if code := <-done; code != http.StatusOK {
			t.Fatalf("status %d, want 200", code)
		}
	}
	if len(held) != 1 || held[0].State != siteapi.HeldRequestHeld || held[0].ModelID != lentModelID || held[0].Lane != cluster.RouteLaneImage {
		t.Fatalf("held = %+v, want only the one request waiting behind the 2-deep backend pipe", held)
	}
}

func TestNodeStateDoesNotListRequestsThatGoStraightToTheBackend(t *testing.T) {
	gate := make(chan struct{})
	service := newLinkedImageService(t, gate, false)
	done := make(chan int, 3)
	for range 3 {
		go func() { done <- postImage(service).Code }()
	}
	waitForActiveRequests(t, service, 3)

	held := service.localNodeState().HeldRequests

	close(gate)
	for range 3 {
		<-done
	}
	if len(held) != 0 {
		t.Fatalf("held = %+v, want nothing for an unlinked model that is never queued", held)
	}
}

func TestNodeStateShowsLentRequestsWithTheirHelper(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	holdNativeImageRequests(scheduler, 4)
	scheduler.acceptOffloadLease(imageLeaseWithSlots(1), queueEventEnqueued)

	held := scheduler.heldRequests(time.Now())

	var lent []siteapi.NodeHeldRequest
	for _, request := range held {
		if request.State == siteapi.HeldRequestLent {
			lent = append(lent, request)
		}
	}
	if len(held) != 2 || len(lent) != 1 || lent[0].HelperNodeID != "master" || lent[0].HelperModelID != "combo-alt-dream" {
		t.Fatalf("held = %+v, want one held and one lent to master/combo-alt-dream", held)
	}
}

func waitForActiveRequests(t *testing.T, service *Service, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(service.localNodeState().ActiveRequests) >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("never saw %d active requests: %+v", want, service.localNodeState().ActiveRequests)
}
