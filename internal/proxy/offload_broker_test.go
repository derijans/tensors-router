package proxy

import (
	"testing"
	"time"

	"tensors-router/internal/analytics"
	"tensors-router/internal/cluster"
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
			ModelID:  "img-" + nodeID,
			Section:  analytics.SectionImage,
			SlopesMS: []float64{jobMS / testJobWork},
			Samples:  100,
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

func costTableWithDecodeOnlySlope(t *testing.T, perJobMS map[string]float64, loadMS map[string]float64) *schedulingcost.Table {
	t.Helper()
	models := map[string]schedulingcost.NodeCosts{}
	for nodeID, jobMS := range perJobMS {
		costs := models[nodeID]
		costs.Models = append(costs.Models, schedulingcost.ModelCost{
			ModelID:  "llm-" + nodeID,
			Section:  analytics.SectionLLM,
			SlopesMS: []float64{0, jobMS / testTextDecodeTokens},
			Samples:  100,
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

const testTextDecodeTokens = 100.0

// candidate models a node running one job with pendingCount more queued behind
// it, which is what an owner under load actually looks like.
func candidate(nodeID string, pendingCount int64, loaded bool) offloadCandidate {
	backlogCount := pendingCount
	if pendingCount > 0 {
		backlogCount++
	}
	work := schedulingcost.ImageWork(testJobWork)
	return offloadCandidate{
		NodeID:            nodeID,
		ModelID:           "img-" + nodeID,
		ConfigFilename:    nodeID + ".kcpps",
		Section:           analytics.SectionImage,
		Loaded:            loaded,
		AcceptingBorrowed: pendingCount == 0,
		PendingCount:      pendingCount,
		PendingWork:       work.Scaled(float64(pendingCount)),
		BacklogCount:      backlogCount,
		BacklogWork:       work.Scaled(float64(backlogCount)),
	}
}

func textCandidateWithContextWindow(nodeID string, pendingCount int64, loaded bool, contextCapacity int) offloadCandidate {
	backlogCount := pendingCount
	if pendingCount > 0 {
		backlogCount++
	}
	work := schedulingcost.TextWork(0, testTextDecodeTokens)
	return offloadCandidate{
		NodeID:            nodeID,
		ModelID:           "llm-" + nodeID,
		ConfigFilename:    nodeID + ".kcpps",
		Section:           analytics.SectionLLM,
		Loaded:            loaded,
		AcceptingBorrowed: pendingCount == 0,
		ContextCapacity:   contextCapacity,
		PendingCount:      pendingCount,
		PendingWork:       work.Scaled(float64(pendingCount)),
		PendingContext:    int64(contextCapacity) * pendingCount,
		BacklogCount:      backlogCount,
		BacklogWork:       work.Scaled(float64(backlogCount)),
	}
}

func linkedHelper(helper offloadCandidate) offloadHelperCandidate {
	return offloadHelperCandidate{offloadCandidate: helper, LoadIfUnloaded: true}
}

func lendingTo(owner offloadCandidate, helpers ...offloadCandidate) lendingOwner {
	candidate := lendingOwner{owner: owner}
	for _, helper := range helpers {
		candidate.helpers = append(candidate.helpers, linkedHelper(helper))
	}
	return candidate
}

func TestLeaseNamesBothModelsAndCarriesTheLinkFlags(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 19000})
	now := time.Now()

	owner := lendingTo(candidate("node-a", 16, true))
	owner.helpers = []offloadHelperCandidate{{offloadCandidate: candidate("node-b", 0, false), LoadIfUnloaded: true, RestoreAfterBorrow: true}}
	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{owner}, costs, now, 30*time.Second)

	want := offloadLease{
		Lane:               cluster.RouteLaneImage,
		OwnerNodeID:        "node-a",
		OwnerModelID:       "img-node-a",
		HelperNodeID:       "node-b",
		HelperModelID:      "img-node-b",
		LoadHelperModel:    true,
		RestoreHelperModel: true,
		ExpiresAt:          now.Add(30 * time.Second),
	}
	if len(leases) != 1 || leases[0] != want {
		t.Fatalf("leases = %+v, want %+v", leases, want)
	}
}

func TestLeaseOmitsRestoreWhenTheLinkDidNotRequestIt(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 19000})

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 16, true), candidate("node-b", 0, false)),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 1 || leases[0].RestoreHelperModel {
		t.Fatalf("leases = %+v, want RestoreHelperModel false when unticked", leases)
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

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 16, true), candidate("node-b", 0, false)),
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

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 1, true), candidate("node-b", 0, false)),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none when the load outlasts the backlog", leases)
	}
}

func TestHelperAlreadyHoldingTheModelPaysNoSwitch(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 19000})

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 1, true), candidate("node-b", 0, true)),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 1 {
		t.Fatalf("leases = %+v, want one for a helper that is already loaded", leases)
	}
}

func TestUnloadedHelperIsSkippedWhenItsLinkForbidsLoading(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 100})
	owner := lendingTo(candidate("node-a", 16, true))
	owner.helpers = []offloadHelperCandidate{{offloadCandidate: candidate("node-b", 0, false), LoadIfUnloaded: false}}

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{owner}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none: the helper would have to load and its link forbids that", leases)
	}
}

func TestLoadedHelperIsLeasedEvenWhenItsLinkForbidsLoading(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		nil)
	owner := lendingTo(candidate("node-a", 16, true))
	owner.helpers = []offloadHelperCandidate{{offloadCandidate: candidate("node-b", 0, true), LoadIfUnloaded: false}}

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{owner}, costs, time.Now(), 30*time.Second)

	if len(leases) != 1 || leases[0].LoadHelperModel {
		t.Fatalf("leases = %+v, want one lease that does not permit a load", leases)
	}
}

// A helper that has never been measured loading this config cannot have its
// switch priced, so it is skipped rather than assumed to load instantly.
func TestHelperWithNoMeasuredLoadIsSkipped(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		nil)

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 16, true), candidate("node-b", 0, false)),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none without a measured load", leases)
	}
}

func TestHelperWithNoMeasuredJobCostIsSkipped(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000},
		map[string]float64{"node-b": 1000})

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 16, true), candidate("node-b", 0, false)),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none for a helper the cost table cannot price", leases)
	}
}

func TestBusyHelperIsNeverLeased(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 100})
	notAccepting := candidate("node-b", 0, true)
	notAccepting.AcceptingBorrowed = false

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 16, true), notAccepting),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none for a helper that is not accepting", leases)
	}
}

func TestSlowerHelperLosesToTheFasterOne(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 20000, "node-c": 6000},
		map[string]float64{"node-b": 1000, "node-c": 1000})

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 16, true), candidate("node-b", 0, false), candidate("node-c", 0, false)),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 1 || leases[0].HelperNodeID != "node-c" {
		t.Fatalf("leases = %+v, want the faster helper", leases)
	}
}

func TestOwnerNeverLendsToANodeItHasNoLinkTo(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 20000, "node-b": 1000},
		map[string]float64{"node-b": 100})

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 16, true)),
		lendingTo(candidate("node-b", 0, true), candidate("node-a", 0, true)),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none: node-a lends to nobody and node-b has nothing queued", leases)
	}
}

// One helper cannot serve two owners at once, because a lease is a single slot.
func TestEachHelperIsLeasedToAtMostOneOwner(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000, "node-c": 8000},
		map[string]float64{"node-c": 1000})

	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-b", 12, true), candidate("node-c", 0, false)),
		lendingTo(candidate("node-a", 16, true), candidate("node-c", 0, false)),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 1 {
		t.Fatalf("leases = %+v, want one", leases)
	}
	if leases[0].OwnerNodeID != "node-a" {
		t.Fatalf("lease went to %q, want the owner with the deeper backlog", leases[0].OwnerNodeID)
	}
}

func TestNoLeasesWithoutQueuedWorkOrAnIdleHelper(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-a": 1000, "node-b": 1000})

	if leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 0, true), candidate("node-b", 0, true)),
	}, costs, time.Now(), 30*time.Second); len(leases) != 0 {
		t.Fatalf("leases = %+v, want none when nothing is queued", leases)
	}

	if leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 5, true), candidate("node-b", 5, true)),
	}, costs, time.Now(), 30*time.Second); len(leases) != 0 {
		t.Fatalf("leases = %+v, want none when the helper is busy too", leases)
	}
}

func TestNilCostTableGrantsNothing(t *testing.T) {
	leases := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 16, true), candidate("node-b", 0, true)),
	}, nil, time.Now(), 30*time.Second)
	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none without a cost table", leases)
	}
}

func imageLeaseFor(ownerNodeID string, ownerModelID string, expiresAt time.Time) offloadLease {
	return offloadLease{Lane: cluster.RouteLaneImage, OwnerNodeID: ownerNodeID, OwnerModelID: ownerModelID, HelperNodeID: "node-b", HelperModelID: "img-node-b", ExpiresAt: expiresAt}
}

func TestLeaseBookExpiresRatherThanRevokes(t *testing.T) {
	book := newOffloadLeaseBook()
	now := time.Now()
	book.Replace([]offloadLease{imageLeaseFor("node-a", "img-node-a", now.Add(30*time.Second))})

	if _, ok := book.Lease(cluster.RouteLaneImage, "node-a", "img-node-a", now); !ok {
		t.Fatal("live lease was not found")
	}
	if _, ok := book.Lease(cluster.RouteLaneImage, "node-a", "img-node-a", now.Add(31*time.Second)); ok {
		t.Fatal("expired lease was still honoured")
	}
}

func TestLeaseBookDropsWhatIsNoLongerPlanned(t *testing.T) {
	book := newOffloadLeaseBook()
	now := time.Now()
	book.Replace([]offloadLease{imageLeaseFor("node-a", "img-node-a", now.Add(30*time.Second))})
	book.Replace(nil)

	if _, ok := book.Lease(cluster.RouteLaneImage, "node-a", "img-node-a", now); ok {
		t.Fatal("lease survived a cycle that no longer planned it")
	}
}

func TestOffloadLeaseBookReplaceAndLeaseAreSafeConcurrently(t *testing.T) {
	book := newOffloadLeaseBook()
	now := time.Now()
	planned := []offloadLease{imageLeaseFor("node-a", "img-node-a", now.Add(30*time.Second))}
	const rounds = 1000
	replaced := make(chan struct{})
	go func() {
		defer close(replaced)
		for range rounds {
			book.Replace(planned)
		}
	}()
	for range rounds {
		book.Lease(cluster.RouteLaneImage, "node-a", "img-node-a", now)
	}
	<-replaced

	if _, ok := book.Lease(cluster.RouteLaneImage, "node-a", "img-node-a", now); !ok {
		t.Fatal("lease installed by the last replace was not found")
	}
}

func TestOffloadLeasesAreKeyedByLaneAndOwnerModel(t *testing.T) {
	book := newOffloadLeaseBook()
	now := time.Now()
	book.Replace([]offloadLease{imageLeaseFor("node-a", "shared-id", now.Add(30*time.Second))})

	if _, ok := book.Lease(cluster.RouteLaneImage, "node-a", "shared-id", now); !ok {
		t.Fatal("image lease not found under its own lane")
	}
	if _, ok := book.Lease(cluster.RouteLaneText, "node-a", "shared-id", now); ok {
		t.Fatal("text lookup found the image lane's lease")
	}
	if _, ok := book.Lease(cluster.RouteLaneImage, "node-a", "other-model", now); ok {
		t.Fatal("a lease for one owner model was honoured for another")
	}
}

func TestTextOffloadSkipsAnUnpricedHelper(t *testing.T) {
	costs := costTableWithDecodeOnlySlope(t,
		map[string]float64{"node-a": 8000},
		map[string]float64{"node-b": 1000})

	leases := planOffloadLeases(cluster.RouteLaneText, []lendingOwner{
		lendingTo(textCandidateWithContextWindow("node-a", 16, true, 8192), textCandidateWithContextWindow("node-b", 0, false, 8192)),
	}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none while the helper is unpriced", leases)
	}
}

func TestTextOffloadHelperMustHoldTheOwnersMeanContext(t *testing.T) {
	costs := costTableWithDecodeOnlySlope(t,
		map[string]float64{"node-a": 8000, "node-b": 1000},
		map[string]float64{"node-b": 100})

	owner := textCandidateWithContextWindow("node-a", 16, true, 32768)
	owner.PendingContext = 6000 * owner.PendingCount
	helper := textCandidateWithContextWindow("node-b", 0, false, 4096)

	leases := planOffloadLeases(cluster.RouteLaneText, []lendingOwner{lendingTo(owner, helper)}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none when the only helper's window is too small", leases)
	}
}

func TestTextOffloadGrantsWhenTheHelperCanHoldTheContext(t *testing.T) {
	costs := costTableWithDecodeOnlySlope(t,
		map[string]float64{"node-a": 8000, "node-b": 1000},
		map[string]float64{"node-b": 100})

	owner := textCandidateWithContextWindow("node-a", 16, true, 32768)
	owner.PendingContext = 6000 * owner.PendingCount
	helper := textCandidateWithContextWindow("node-b", 0, false, 8192)

	leases := planOffloadLeases(cluster.RouteLaneText, []lendingOwner{lendingTo(owner, helper)}, costs, time.Now(), 30*time.Second)

	if len(leases) != 1 {
		t.Fatalf("leases = %+v, want one when the helper's window fits", leases)
	}
}

func TestUnloadedTextHelperIsSkippedWhenItsLinkForbidsLoading(t *testing.T) {
	costs := costTableWithDecodeOnlySlope(t,
		map[string]float64{"node-a": 8000, "node-b": 1000},
		map[string]float64{"node-b": 100})
	owner := lendingTo(textCandidateWithContextWindow("node-a", 16, true, 8192))
	owner.helpers = []offloadHelperCandidate{{offloadCandidate: textCandidateWithContextWindow("node-b", 0, false, 8192), LoadIfUnloaded: false}}

	leases := planOffloadLeases(cluster.RouteLaneText, []lendingOwner{owner}, costs, time.Now(), 30*time.Second)

	if len(leases) != 0 {
		t.Fatalf("leases = %+v, want none: the text helper would have to load and its link forbids that", leases)
	}
}

func TestMeanWorkIsElementwise(t *testing.T) {
	summed := schedulingcost.TextWork(800, 200)
	mean := summed.Mean(4)
	if mean.Term(0) != 200 || mean.Term(1) != 50 {
		t.Fatalf("mean = %+v, want (200, 50)", mean)
	}
}
