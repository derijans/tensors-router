package proxy

import (
	"time"

	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/schedulingcost"
)

const (
	fasterHelperSlots = 2
	slowerHelperSlots = 1
	probeHelperSlots  = 1
)

const (
	reasonHelperNotAccepting = "helper_not_accepting"
	reasonHelperClaimed      = "helper_claimed"
	reasonLoadForbidden      = "load_forbidden"
	reasonContextTooSmall    = "context_too_small"
	reasonSwitchUnpriced     = "switch_unpriced"
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
	return evaluation.switchMS + evaluation.serviceMS
}

func evaluateHelper(helper offloadHelperCandidate, claimed map[string]bool, costs *schedulingcost.Table, demand ownerDemand) helperEvaluation {
	evaluation := helperEvaluation{helper: helper, ineligible: helperIneligibility(helper, claimed, demand.meanContext)}
	switchMS, switchPriced := offloadSwitchMS(helper.offloadCandidate, costs)
	serviceMS, serviceSource, servicePriced := helperServiceMS(helper.offloadCandidate, costs, demand)
	evaluation.switchMS, evaluation.serviceMS, evaluation.serviceSource = switchMS, serviceMS, serviceSource
	switch {
	case !switchPriced:
		evaluation.unpriced = reasonSwitchUnpriced
	case !servicePriced:
		evaluation.unpriced = reasonServiceUnpriced
	}
	return evaluation
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
		if !evaluation.mayServe() || evaluation.helper.IdleFor < probeIdle {
			continue
		}
		if best < 0 || evaluation.helper.IdleFor > evaluations[best].helper.IdleFor ||
			(evaluation.helper.IdleFor == evaluations[best].helper.IdleFor && evaluation.helper.NodeID < evaluations[best].helper.NodeID) {
			best = index
		}
	}
	return best
}

func soonestProbeFor(helpers []offloadHelperCandidate, claimed map[string]bool, meanContext int, probeIdle time.Duration) time.Duration {
	var soonest time.Duration
	for _, helper := range helpers {
		if helperIneligibility(helper, claimed, meanContext) != "" || helper.IdleFor >= probeIdle {
			continue
		}
		if wait := probeIdle - helper.IdleFor; soonest == 0 || wait < soonest {
			soonest = wait
		}
	}
	return soonest
}

func helperSlotsFor(evaluation helperEvaluation, demand ownerDemand, probe bool) int {
	switch {
	case probe:
		return probeHelperSlots
	case evaluation.serviceMS <= demand.jobMS:
		return fasterHelperSlots
	default:
		return slowerHelperSlots
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

func planDecision(lane string, trigger string, demand ownerDemand, evaluation helperEvaluation, chosen bool, probe bool) offloaddecisions.Record {
	record := offloaddecisions.Record{
		Kind:          offloaddecisions.KindPlan,
		Trigger:       trigger,
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
	record.Slots = helperSlotsFor(evaluation, demand, probe)
	if probe {
		record.Outcome = offloaddecisions.OutcomeProbe
		return record
	}
	record.Outcome = offloaddecisions.OutcomeGranted
	record.Reason = ""
	return record
}
