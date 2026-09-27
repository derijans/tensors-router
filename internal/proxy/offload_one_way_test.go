package proxy

import (
	"net/http"
	"testing"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/routinggroups"
)

func newHelperOnlyImageService(t *testing.T) (*Service, *recordedDecisions) {
	t.Helper()
	service := newLinkedImageService(t, nil, false)
	service.clusterRole = cluster.RoleMaster
	service.installRoutingLinks(routingLinkSnapshot{Image: []routinggroups.Link{{
		Owner:          routinggroups.Endpoint{NodeID: "slave-a", ModelID: "combo-alt-dream"},
		Helper:         routinggroups.Endpoint{NodeID: service.nodeID, ModelID: lentModelID},
		LoadIfUnloaded: true,
	}}})
	recorded := &recordedDecisions{}
	service.scheduler.decisions = recorded
	return service, recorded
}

func TestHelperOnlyModelRefusesALeaseNamingItAsOwner(t *testing.T) {
	service, recorded := newHelperOnlyImageService(t)
	entries := holdNativeImageRequests(service.scheduler, 4)

	service.scheduler.acceptOffloadLease(imageLeaseWithSlots(2), queueEventEnqueued)

	if withdrawn := withdrawnEntries(entries); len(withdrawn) != 0 {
		t.Fatalf("withdrew %d of the helper's own requests, want none lent from a helper-only model", len(withdrawn))
	}
	if _, held := service.scheduler.activeOffloadLease(cluster.RouteLaneImage, lentModelID, imageLeaseWithSlots(2).ExpiresAt.Add(-1)); held {
		t.Fatal("helper-only model kept a lease, want it refused")
	}
	if refused := recorded.withOutcome(offloaddecisions.OutcomeLeaseRefused); len(refused) != 1 || refused[0].Reason != leaseRefusedNotAnOwner {
		t.Fatalf("lease refusals = %+v, want one not_an_owner refusal", refused)
	}
}

func TestHelperOnlyModelServesItsOwnRequestsWithoutLendingState(t *testing.T) {
	service, recorded := newHelperOnlyImageService(t)

	if recorder := postImage(service); recorder.Code != http.StatusOK {
		t.Fatalf("native request to the helper-only model status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorded.mu.Lock()
	defer recorded.mu.Unlock()
	for _, record := range recorded.records {
		if record.OwnerModelID == lentModelID {
			t.Fatalf("helper-only model appeared as an owner in %+v", record)
		}
	}
}
