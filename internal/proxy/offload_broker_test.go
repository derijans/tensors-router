package proxy

import (
	"testing"
	"time"

	"tensors-router/internal/analytics"
	"tensors-router/internal/schedulingcost"
)

const testJobWork = 30 * 1024 * 1024

// costTableFor builds a table directly rather than through a fit, so each test
// states the per-job cost and load cost it is reasoning about.
func costTableFor(t *testing.T, perJobMS map[string]float64, loadMS map[string]float64) *schedulingcost.Table {
	t.Helper()
	models := map[string]schedulingcost.NodeCosts{}
	for nodeID, jobMS := range perJobMS {
		costs := models[nodeID]
		costs.Models = append(costs.Models, schedulingcost.ModelCost{
			ModelID: "img-" + nodeID,
			Section: analytics.SectionImage,
			SlopeMS: jobMS / testJobWork,
			Samples: 100,
		})
		models[nodeID] = costs
	}
	for nodeID, load := range loadMS {
		costs := models[nodeID]
		costs.Loads = append(costs.Loads, schedulingcost.LoadCost{ConfigFilename: nodeID + ".kcpps", LoadMS: load})
		models[nodeID] = costs
	}
	return schedulingcost.Merge(models)
}

// candidate models a node running one job with pendingCount more queued behind
// it, which is what an owner under load actually looks like.
func candidate(nodeID string, pendingCount int64, loaded bool) offloadCandidate {
	backlogCount := pendingCount
	if pendingCount > 0 {
		backlogCount++
	}
	return offloadCandidate{
		NodeID:            nodeID,
		ModelID:           "img-" + nodeID,
		ConfigFilename:    nodeID + ".kcpps",
		Loaded:            loaded,
		AcceptingBorrowed: pendingCount == 0,
		PendingCount:      pendingCount,
		PendingWork:       float64(pendingCount) * testJobWork,
		BacklogCount:      backlogCount,
		BacklogWork:       float64(backlogCount) * testJobWork,
	}
}

// The idle node has to load the model first. With a deep backlog on the owner
// that load is still worth paying, because it amortises over every job that then
// flows through the slot.
func TestLeaseIsGrantedWhenTheLoadFitsUnderTheBacklog(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 19000})
	now := time.Now()

	leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 16, true),
		candidate("node-b", 0, false),
	}, costs, now, 30*time.Second)

	if len(leases) != 1 {
		t.Fatalf("leases = %+v, want one", leases)
	}
	if leases[0].OwnerNodeID != "node-a" || leases[0].HelperNodeID != "node-b" {
		t.Fatalf("unexpected lease %+v", leases[0])
	}
	if !leases[0].ExpiresAt.After(now) {
		t.Fatal("lease was issued already expired")
	}
}

// The same pair with a shallow backlog. This is the case that proves the load is
// genuinely weighed rather than treated as a discount: nothing about the nodes
// changed, only the size of the queue.
func TestLeaseIsRefusedWhenTheLoadCostsMoreThanTheBacklog(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 19000})

	leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 1, true),
		candidate("node-b", 0, false),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none when the load outlasts the backlog", leases)
	}
}

func TestHelperAlreadyHoldingTheModelPaysNoSwitch(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 19000})

	leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 1, true),
		candidate("node-b", 0, true),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 1 {
		t.Fatalf("leases = %+v, want one for a helper that is already loaded", leases)
	}
}

// A helper that has never been measured loading this config cannot have its
// switch priced, so it is skipped rather than assumed to load instantly.
func TestHelperWithNoMeasuredLoadIsSkipped(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		nil)

	leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 16, true),
		candidate("node-b", 0, false),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none without a measured load", leases)
	}
}

// Until every member has history the group keeps the existing rotation, which is
// what lets the unqualified members build the history they are missing.
func TestGroupIsSkippedWhileAnyMemberIsUnqualified(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000},
		map[string]float64{"node-b": 1000})

	leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 16, true),
		candidate("node-b", 0, false),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none while a member is unqualified", leases)
	}
}

func TestBusyHelperIsNeverLeased(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 100})
	notAccepting := candidate("node-b", 0, true)
	notAccepting.AcceptingBorrowed = false

	leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 16, true),
		notAccepting,
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none for a helper that is not accepting", leases)
	}
}

func TestSlowerHelperLosesToTheFasterOne(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 20000, "node-c": 6000},
		map[string]float64{"node-b": 1000, "node-c": 1000})

	leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 16, true),
		candidate("node-b", 0, false),
		candidate("node-c", 0, false),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 1 || leases[0].HelperNodeID != "node-c" {
		t.Fatalf("leases = %+v, want the faster helper", leases)
	}
}

// One helper cannot serve two owners at once, because a lease is a single slot.
func TestEachHelperIsLeasedToAtMostOneOwner(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000, "node-c": 8000},
		map[string]float64{"node-c": 1000})

	leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 16, true),
		candidate("node-b", 12, true),
		candidate("node-c", 0, false),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 1 {
		t.Fatalf("leases = %+v, want one", leases)
	}
	if leases[0].OwnerNodeID != "node-a" {
		t.Fatalf("lease went to %q, want the owner with the deeper backlog", leases[0].OwnerNodeID)
	}
}

func TestNoLeasesWithoutAHelperOrAnOwner(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-a": 1000, "node-b": 1000})

	if leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 0, true),
		candidate("node-b", 0, true),
	}, costs, time.Now(), 30*time.Second); len(leases) != 0 {
		t.Fatalf("leases = %+v, want none when nothing is queued", leases)
	}

	if leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 5, true),
		candidate("node-b", 5, true),
	}, costs, time.Now(), 30*time.Second); len(leases) != 0 {
		t.Fatalf("leases = %+v, want none when every member is busy", leases)
	}
}

func TestNilCostTableGrantsNothing(t *testing.T) {
	leases := planOffloadLeases("group", []offloadCandidate{
		candidate("node-a", 16, true),
		candidate("node-b", 0, true),
	}, nil, time.Now(), 30*time.Second)
	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none without a cost table", leases)
	}
}

func TestLeaseBookExpiresRatherThanRevokes(t *testing.T) {
	book := newOffloadLeaseBook()
	now := time.Now()
	book.Replace([]offloadLease{{GroupID: "group", OwnerNodeID: "node-a", HelperNodeID: "node-b", ExpiresAt: now.Add(30 * time.Second)}})

	if _, ok := book.Lease("group", "node-a", now); !ok {
		t.Fatal("live lease was not found")
	}
	if _, ok := book.Lease("group", "node-a", now.Add(31*time.Second)); ok {
		t.Fatal("expired lease was still honoured")
	}
}

func TestLeaseBookDropsWhatIsNoLongerPlanned(t *testing.T) {
	book := newOffloadLeaseBook()
	now := time.Now()
	book.Replace([]offloadLease{{GroupID: "group", OwnerNodeID: "node-a", HelperNodeID: "node-b", ExpiresAt: now.Add(30 * time.Second)}})
	book.Replace(nil)

	if _, ok := book.Lease("group", "node-a", now); ok {
		t.Fatal("lease survived a cycle that no longer planned it")
	}
}
