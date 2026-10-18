package proxy

import (
	"slices"
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/schedulingcost"
)

func helperOnAnotherBackendFamily(idle time.Duration) offloadCandidate {
	helper := helperHoldingAnotherModel(idle)
	helper.SwitchesBackendFamily = true
	return helper
}

func TestHelperOnAnotherBackendFamilyIsNeverProbed(t *testing.T) {
	plan := planOffloadLeases(cluster.RouteLaneImage, []lendingOwner{
		lendingTo(candidate("node-a", 1, true), helperOnAnotherBackendFamily(time.Hour)),
	}, schedulingcost.Merge(nil), time.Now(), testPlanPolicy)

	if len(plan.leases) != 0 {
		t.Fatalf("leases = %+v, want no unpriced probe that would stop the helper's whole backend family", plan.leases)
	}
	if decision := decisionFor(t, plan, "node-b"); decision.Outcome != offloaddecisions.OutcomeSkipped || decision.Reason != reasonFamilySwitchUnpriced {
		t.Fatalf("decision = %+v, want a skip for %s", decision, reasonFamilySwitchUnpriced)
	}
	if plan.nextProbeDue != 0 {
		t.Fatalf("next probe due in %v, want none for a helper that is never probed", plan.nextProbeDue)
	}
}

func TestHelperOnAnotherBackendFamilyIsLeasedWhenTheSwitchPays(t *testing.T) {
	plan := planAgainstBusyOwner(costTableWithDisplacedLoad(5000), helperOnAnotherBackendFamily(time.Hour))

	if len(plan.leases) != 1 || plan.leases[0].Probe {
		t.Fatalf("leases = %+v, want one priced lease", plan.leases)
	}
}

func TestHelperOnAnotherBackendFamilyIsNotProbedWhenTheSwitchCostsMoreThanTheBacklog(t *testing.T) {
	plan := planAgainstBusyOwner(costTableWithDisplacedLoad(120000), helperOnAnotherBackendFamily(time.Hour))

	if len(plan.leases) != 0 {
		t.Fatalf("leases = %+v, want none: the cost rule refused and a family switch is never probed", plan.leases)
	}
	if decision := decisionFor(t, plan, "node-b"); decision.Reason != reasonCostRejected {
		t.Fatalf("decision = %+v, want %s", decision, reasonCostRejected)
	}
}

func TestFamilySwitchAlsoDisplacesTheTranscriptionConfig(t *testing.T) {
	status := NodeRuntimeStatus{ActiveTextConfig: "qwen.kcpps", ActiveSTTConfigFilename: "whisper.kcpps"}

	if got := displacedConfigsOf("gemma.kcpps", status, false); slices.Contains(got, "whisper.kcpps") {
		t.Fatalf("displaced = %v, want the transcription config kept when the family stays", got)
	}
	if got := displacedConfigsOf("gemma.kcpps", status, true); !slices.Contains(got, "whisper.kcpps") || !slices.Contains(got, "qwen.kcpps") {
		t.Fatalf("displaced = %v, want the text and transcription configs the family switch stops", got)
	}
}

func TestSwitchesBackendFamilyComparesTheModelWithTheNodesRunningFamily(t *testing.T) {
	koboldModel := cluster.Model{BackendMode: cluster.BackendModeKobold}

	if !switchesBackendFamily(koboldModel, NodeRuntimeStatus{BackendMode: cluster.BackendModeLlamaSDCPP}) {
		t.Fatal("expected a kobold model on a llama_sdcpp node to switch families")
	}
	if switchesBackendFamily(koboldModel, NodeRuntimeStatus{BackendMode: cluster.BackendModeKobold}) {
		t.Fatal("expected a kobold model on a kobold node to stay in its family")
	}
	if switchesBackendFamily(koboldModel, NodeRuntimeStatus{}) {
		t.Fatal("expected no switch when the node reports no family")
	}
}
