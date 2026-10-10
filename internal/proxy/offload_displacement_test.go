package proxy

import (
	"testing"
	"time"

	"tensors-router/internal/analytics"
	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/schedulingcost"
)

const (
	helperOwnConfig    = "node-b-own.kcpps"
	displacementJobMS  = 8000
	displacementLoadMS = 19000
)

func costTableWithDisplacedLoad(displacedLoadMS float64) *schedulingcost.Table {
	loads := []schedulingcost.LoadCost{{ConfigFilename: "node-b.kcpps", LoadMS: displacementLoadMS}}
	if displacedLoadMS > 0 {
		loads = append(loads, schedulingcost.LoadCost{ConfigFilename: helperOwnConfig, LoadMS: displacedLoadMS})
	}
	model := func(nodeID string) schedulingcost.NodeCosts {
		return schedulingcost.NodeCosts{Models: []schedulingcost.ModelCost{{
			ModelID: "img-" + nodeID, Section: analytics.SectionImage, SlopesMS: []float64{float64(displacementJobMS) / testJobWork}, Samples: 100,
		}}}
	}
	helperCosts := model("node-b")
	helperCosts.Loads = loads
	return schedulingcost.Merge(map[string]schedulingcost.NodeCosts{"node-a": model("node-a"), "node-b": helperCosts})
}

func helperHoldingAnotherModel(idle time.Duration) offloadCandidate {
	helper := candidate("node-b", 0, false)
	helper.DisplacedConfigs = []string{helperOwnConfig}
	helper.IdleFor = idle
	return helper
}

func planAgainstBusyOwner(costs *schedulingcost.Table, helper offloadCandidate) offloadPlan {
	return planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 16, true), helper),
	}, costs, time.Now(), testPlanPolicy)
}

func requireNoPricedLease(t *testing.T, plan offloadPlan, why string) {
	t.Helper()
	for _, lease := range plan.leases {
		if !lease.Probe {
			t.Fatalf("lease = %+v, want none priced: %s", lease, why)
		}
	}
	if decision := decisionFor(t, plan, "node-b"); decision.Outcome == offloaddecisions.OutcomeGranted {
		t.Fatalf("decision = %+v, want it not granted: %s", decision, why)
	}
}

func TestHelperIsNotDisplacedWhenGettingItsOwnModelBackCostsMoreThanTheBacklog(t *testing.T) {
	costs := costTableWithDisplacedLoad(120000)

	plan := planAgainstBusyOwner(costs, helperHoldingAnotherModel(time.Hour))

	requireNoPricedLease(t, plan, "the helper would spend longer restoring its own model than the owner needs to drain")
	if decision := decisionFor(t, plan, "node-b"); decision.Reason != reasonCostRejected {
		t.Fatalf("decision = %+v, want the restore cost to be what the rule refused on", decision)
	}
}

func TestHelperIsDisplacedWhenTheDetourStillPaysOff(t *testing.T) {
	costs := costTableWithDisplacedLoad(5000)

	plan := planAgainstBusyOwner(costs, helperHoldingAnotherModel(time.Hour))

	if len(plan.leases) != 1 || plan.leases[0].Probe {
		t.Fatalf("leases = %+v, want one priced lease", plan.leases)
	}
	if got, want := decisionFor(t, plan, "node-b").SwitchMS, float64(displacementLoadMS); got != want {
		t.Fatalf("switch_ms = %v, want %v", got, want)
	}
}

func TestHelperThatCannotPriceGettingItsOwnModelBackIsNotDisplaced(t *testing.T) {
	costs := costTableWithDisplacedLoad(0)

	plan := planAgainstBusyOwner(costs, helperHoldingAnotherModel(time.Hour))

	requireNoPricedLease(t, plan, "the restore was never measured")
	if decision := decisionFor(t, plan, "node-b"); decision.Reason != reasonRestoreUnpriced {
		t.Fatalf("decision = %+v, want %s", decision, reasonRestoreUnpriced)
	}
}

func TestHelperThatServedItsOwnWorkRecentlyKeepsItsModel(t *testing.T) {
	costs := costTableWithDisplacedLoad(5000)
	detour := (displacementLoadMS + 5000) * time.Millisecond

	busy := planAgainstBusyOwner(costs, helperHoldingAnotherModel(detour-time.Second))
	settled := planAgainstBusyOwner(costs, helperHoldingAnotherModel(detour+time.Second))

	if len(busy.leases) != 0 {
		t.Fatalf("leases = %+v, want none while the helper's own traffic may still return", busy.leases)
	}
	if decision := decisionFor(t, busy, "node-b"); decision.Reason != reasonHelperRecentlyBusy || decision.Outcome != offloaddecisions.OutcomeSkipped {
		t.Fatalf("decision = %+v, want a skip for %s", decision, reasonHelperRecentlyBusy)
	}
	if len(settled.leases) != 1 {
		t.Fatalf("leases = %+v, want one once the helper has been idle longer than the detour", settled.leases)
	}
}

func TestHelperThatAlreadyHoldsItsModelIsNeverHeldBackByRecentTraffic(t *testing.T) {
	costs := costTableWithDisplacedLoad(120000)
	helper := candidate("node-b", 0, true)
	helper.DisplacedConfigs = []string{helperOwnConfig}
	helper.IdleFor = 0

	plan := planAgainstBusyOwner(costs, helper)

	if len(plan.leases) != 1 {
		t.Fatalf("leases = %+v, want one: nothing is displaced, so there is nothing to restore and no detour", plan.leases)
	}
}

func TestUnloadedHelperIdleOnlyPastTheProbeThresholdIsNotProbed(t *testing.T) {
	costs := costTableFor(t,
		map[string]float64{"node-a": 8000, "node-b": 8000},
		map[string]float64{"node-b": 19000})
	helper := candidate("node-b", 0, false)
	helper.IdleFor = 6 * time.Second

	plan := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 1, true), helper),
	}, costs, time.Now(), testPlanPolicy)

	if len(plan.leases) != 0 {
		t.Fatalf("leases = %+v, want no probe: swapping the helper's model is not worth an unpriced experiment after 6s", plan.leases)
	}
}

func TestDisplacedConfigsNameEverythingTheNodeHoldsExceptTheHelpersOwnConfig(t *testing.T) {
	status := NodeRuntimeStatus{ActiveTextConfig: "qwen.kcpps", ActiveImageConfig: "krea.kcpps"}

	if got := displacedConfigsOf("krea.kcpps", status); len(got) != 1 || got[0] != "qwen.kcpps" {
		t.Fatalf("displaced = %v, want only the text config", got)
	}
	if got := displacedConfigsOf("gemma.kcpps", status); len(got) != 2 {
		t.Fatalf("displaced = %v, want both configs", got)
	}
	if got := displacedConfigsOf("qwen.kcpps", NodeRuntimeStatus{ActiveTextConfig: "qwen.kcpps"}); len(got) != 0 {
		t.Fatalf("displaced = %v, want nothing when the helper's model is already loaded", got)
	}
}
