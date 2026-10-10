package proxy

import (
	"time"

	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/schedulingcost"
)

const (
	reasonHelperNotAccepting = "helper_not_accepting"
	reasonHelperClaimed      = "helper_claimed"
	reasonLoadForbidden      = "load_forbidden"
	reasonContextTooSmall    = "context_too_small"
	reasonSwitchUnpriced     = "switch_unpriced"
	reasonRestoreUnpriced    = "restore_unpriced"
	reasonHelperRecentlyBusy = "helper_recently_busy"
	reasonServiceUnpriced    = "service_unpriced"
	reasonOwnerUnpriced      = "owner_unpriced"
	reasonCostRejected       = "cost_rejected"
	reasonOutranked          = "outranked"
)

type ownerDemand struct {
	owner       offloadCandidate
	meanWork    schedulingcost.Work
	meanContext int
	keepMS      float64
	jobMS       float64
	priced      bool
}

func ownerDemandFor(owner offloadCandidate, costs *schedulingcost.Table) ownerDemand {
	key := offloadModelKey(owner)
	demand := ownerDemand{
		owner:       owner,
		meanWork:    owner.PendingWork.Mean(owner.PendingCount),
		meanContext: int(owner.PendingContext / owner.PendingCount),
	}
	keepMS, keepPriced := costs.PredictQueueMS(key, owner.BacklogCount, owner.BacklogWork)
	jobMS, jobPriced := costs.PredictMS(key, demand.meanWork)
	demand.keepMS, demand.jobMS, demand.priced = keepMS, jobMS, keepPriced && jobPriced
	return demand
}

type helperEvaluation struct {
	helper        offloadHelperCandidate
	ineligible    string
	unpriced      string
	switchMS      float64
	restoreMS     float64
	serviceMS     float64
	serviceSource string
}

func (evaluation helperEvaluation) mayServe() bool {
	return evaluation.ineligible == ""
}

func (evaluation helperEvaluation) priced() bool {
	return evaluation.mayServe() && evaluation.unpriced == ""
}

func (evaluation helperEvaluation) totalMS() float64 {
	return evaluation.switchMS + evaluation.restoreMS + evaluation.serviceMS
}

func evaluateHelper(helper offloadHelperCandidate, claimed map[string]bool, costs *schedulingcost.Table, demand ownerDemand) helperEvaluation {
	evaluation := helperEvaluation{helper: helper, ineligible: helperIneligibility(helper, claimed, demand.meanContext)}
	switchMS, switchPriced := offloadSwitchMS(helper.offloadCandidate, costs)
	restoreMS, restorePriced := offloadRestoreMS(helper.offloadCandidate, costs)
	serviceMS, serviceSource, servicePriced := helperServiceMS(helper.offloadCandidate, costs, demand)
	evaluation.switchMS, evaluation.restoreMS, evaluation.serviceMS, evaluation.serviceSource = switchMS, restoreMS, serviceMS, serviceSource
	switch {
	case !switchPriced:
		evaluation.unpriced = reasonSwitchUnpriced
	case !restorePriced:
		evaluation.unpriced = reasonRestoreUnpriced
	case !servicePriced:
		evaluation.unpriced = reasonServiceUnpriced
	}
	if evaluation.priced() && helperStillNeedsItsOwnModel(helper.offloadCandidate, switchMS+restoreMS) {
		evaluation.ineligible = reasonHelperRecentlyBusy
	}
	return evaluation
}

func helperStillNeedsItsOwnModel(helper offloadCandidate, detour float64) bool {
	return !helper.Loaded && helper.IdleFor < durationFromMilliseconds(detour)
}

func offloadRestoreMS(helper offloadCandidate, costs *schedulingcost.Table) (float64, bool) {
	if helper.Loaded {
		return 0, true
	}
	var total float64
	for _, displaced := range helper.DisplacedConfigs {
		loadMS, priced := costs.LoadMS(schedulingcost.LoadKey{NodeID: helper.NodeID, ConfigFilename: displaced})
		if !priced {
			return 0, false
		}
		total += loadMS
	}
	return total, true
}

func helperIneligibility(helper offloadHelperCandidate, claimed map[string]bool, meanContext int) string {
	switch {
	case !helper.AcceptingBorrowed:
		return reasonHelperNotAccepting
	case claimed[helper.NodeID]:
		return reasonHelperClaimed
	case !helper.Loaded && !helper.LoadIfUnloaded:
		return reasonLoadForbidden
	case !helperWindowHolds(helper.offloadCandidate, meanContext):
		return reasonContextTooSmall
	default:
		return ""
	}
}

func helperWindowHolds(helper offloadCandidate, meanContext int) bool {
	return meanContext <= 0 || helper.ContextCapacity >= meanContext
}

func offloadSwitchMS(helper offloadCandidate, costs *schedulingcost.Table) (float64, bool) {
	if helper.Loaded {
		return 0, true
	}
	return costs.LoadMS(schedulingcost.LoadKey{NodeID: helper.NodeID, ConfigFilename: helper.ConfigFilename})
}

func helperServiceMS(helper offloadCandidate, costs *schedulingcost.Table, demand ownerDemand) (float64, string, bool) {
	if serviceMS, priced := costs.PredictMS(offloadModelKey(helper), demand.meanWork); priced {
		return serviceMS, offloaddecisions.ServiceSourceHelper, true
	}
	if demand.priced {
		return demand.jobMS, offloaddecisions.ServiceSourceOwnerFallback, true
	}
	return 0, "", false
}

func chooseHelper(evaluations []helperEvaluation, demand ownerDemand, probeIdle time.Duration) (chosen int, probe bool) {
	if paying := cheapestPayingHelper(evaluations, demand); paying >= 0 {
		return paying, false
	}
	return longestIdleHelper(evaluations, probeIdle), true
}

func cheapestPayingHelper(evaluations []helperEvaluation, demand ownerDemand) int {
	best := -1
	if !demand.priced {
		return best
	}
	for index, evaluation := range evaluations {
		if !evaluation.priced() || evaluation.totalMS() >= demand.keepMS {
			continue
		}
		if best < 0 || evaluation.totalMS() < evaluations[best].totalMS() ||
			(evaluation.totalMS() == evaluations[best].totalMS() && evaluation.helper.NodeID < evaluations[best].helper.NodeID) {
			best = index
		}
	}
	return best
}

func longestIdleHelper(evaluations []helperEvaluation, probeIdle time.Duration) int {
	best := -1
	if probeIdle <= 0 {
		return best
	}
	for index, evaluation := range evaluations {
		if !evaluation.mayServe() || evaluation.helper.IdleFor < probeIdleBar(evaluation.helper.offloadCandidate, probeIdle) {
			continue
		}
		if best < 0 || evaluation.helper.IdleFor > evaluations[best].helper.IdleFor ||
			(evaluation.helper.IdleFor == evaluations[best].helper.IdleFor && evaluation.helper.NodeID < evaluations[best].helper.NodeID) {
			best = index
		}
	}
	return best
}

const unloadedProbeIdleFactor = 12

func probeIdleBar(helper offloadCandidate, probeIdle time.Duration) time.Duration {
	if helper.Loaded {
		return probeIdle
	}
	return probeIdle * unloadedProbeIdleFactor
}

func soonestProbeFor(helpers []offloadHelperCandidate, claimed map[string]bool, meanContext int, probeIdle time.Duration) time.Duration {
	var soonest time.Duration
	for _, helper := range helpers {
		bar := probeIdleBar(helper.offloadCandidate, probeIdle)
		if helperIneligibility(helper, claimed, meanContext) != "" || helper.IdleFor >= bar {
			continue
		}
		if wait := bar - helper.IdleFor; soonest == 0 || wait < soonest {
			soonest = wait
		}
	}
	return soonest
}

type helperSlots struct {
	faster int
	slower int
	probe  int
}

func (slots helperSlots) forHelper(evaluation helperEvaluation, demand ownerDemand, probe bool) int {
	switch {
	case probe:
		return slots.probe
	case evaluation.serviceMS <= demand.jobMS:
		return slots.faster
	default:
		return slots.slower
	}
}

func costRuleRejection(evaluation helperEvaluation, demand ownerDemand) string {
	switch {
	case !evaluation.mayServe():
		return evaluation.ineligible
	case !demand.priced:
		return reasonOwnerUnpriced
	case !evaluation.priced():
		return evaluation.unpriced
	case evaluation.totalMS() >= demand.keepMS:
		return reasonCostRejected
	default:
		return reasonOutranked
	}
}

func planDecision(lane string, policy offloadPlanPolicy, demand ownerDemand, evaluation helperEvaluation, chosen bool, probe bool) offloaddecisions.Record {
	record := offloaddecisions.Record{
		Kind:          offloaddecisions.KindPlan,
		Trigger:       policy.trigger,
		Lane:          lane,
		OwnerNodeID:   demand.owner.NodeID,
		OwnerModelID:  demand.owner.ModelID,
		HelperNodeID:  evaluation.helper.NodeID,
		HelperModelID: evaluation.helper.ModelID,
		Outcome:       offloaddecisions.OutcomeSkipped,
		Reason:        costRuleRejection(evaluation, demand),
		PendingCount:  demand.owner.PendingCount,
		BacklogCount:  demand.owner.BacklogCount,
		KeepMS:        demand.keepMS,
		SwitchMS:      evaluation.switchMS,
		ServiceMS:     evaluation.serviceMS,
		OwnerJobMS:    demand.jobMS,
		ServiceSource: evaluation.serviceSource,
		HelperIdleMS:  evaluation.helper.IdleFor.Milliseconds(),
	}
	if !chosen {
		return record
	}
	record.Slots = policy.slots.forHelper(evaluation, demand, probe)
	if probe {
		record.Outcome = offloaddecisions.OutcomeProbe
		return record
	}
	record.Outcome = offloaddecisions.OutcomeGranted
	record.Reason = ""
	return record
}
