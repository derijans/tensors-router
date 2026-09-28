package proxy

import (
	"time"

	"tensors-router/internal/offloaddecisions"
)

const reasonHelperFinishesSooner = "helper_finishes_sooner"

func (scheduler *scheduler) installAdmissionHolds() {
	for _, lane := range lendingLanes {
		scheduler.queueForLane(lane).SetAdmissionHold(scheduler.holdForFasterHelper(lane))
	}
}

func (scheduler *scheduler) holdForFasterHelper(lane string) admissionHold {
	return func(next *offloadEntry, pipeline ownerPipeline) bool {
		if !scheduler.currentSettings().HoldForFasterHelper {
			return false
		}
		now := time.Now()
		lease, live := scheduler.activeOffloadLease(lane, next.modelID, now)
		if !live || !lease.knowsBothSpeeds() {
			return false
		}
		lent := scheduler.lent.InFlight(lane, next.modelID)
		if len(lent) < lease.slots() {
			return false
		}
		ownerFinish := pipeline.nextJobFinish(now, lease.OwnerJobMS)
		helperFinish := lease.nextJobFinishAfter(lent, now)
		if !helperFinish.Before(ownerFinish) {
			return false
		}
		if !next.heldForHelper {
			next.heldForHelper = true
			scheduler.record(offloaddecisions.Record{
				Kind:          offloaddecisions.KindDispatch,
				Lane:          lane,
				OwnerNodeID:   lease.OwnerNodeID,
				OwnerModelID:  lease.OwnerModelID,
				HelperNodeID:  lease.HelperNodeID,
				HelperModelID: lease.HelperModelID,
				Outcome:       offloaddecisions.OutcomeHeldForHelper,
				Reason:        reasonHelperFinishesSooner,
				KeepMS:        float64(ownerFinish.Sub(now).Milliseconds()),
				SwitchMS:      lease.HelperSwitchMS,
				ServiceMS:     lease.HelperServiceMS,
				OwnerJobMS:    lease.OwnerJobMS,
				Slots:         lease.slots(),
				LentOut:       len(lent),
				WaitMS:        now.Sub(next.arrived).Milliseconds(),
			})
		}
		return true
	}
}

func (pipeline ownerPipeline) nextJobFinish(now time.Time, ownerJobMS float64) time.Time {
	job := durationFromMilliseconds(ownerJobMS)
	generatingDone := pipeline.generatingSince.Add(job)
	if generatingDone.Before(now) {
		generatingDone = now
	}
	queuedBehind := time.Duration(pipeline.inFlight-1) * job
	return generatingDone.Add(queuedBehind).Add(job)
}

func (lease offloadLease) nextJobFinishAfter(lent []lentRequest, now time.Time) time.Time {
	service := durationFromMilliseconds(lease.HelperServiceMS)
	busyUntil := lent[0].lentAt.Add(durationFromMilliseconds(lease.HelperSwitchMS))
	for _, request := range lent {
		if request.lentAt.After(busyUntil) {
			busyUntil = request.lentAt
		}
		busyUntil = busyUntil.Add(service)
	}
	if busyUntil.Before(now) {
		busyUntil = now
	}
	return busyUntil.Add(service)
}

func durationFromMilliseconds(milliseconds float64) time.Duration {
	return time.Duration(milliseconds * float64(time.Millisecond))
}
