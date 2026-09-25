package proxy

import (
	"time"

	"tensors-router/internal/offloaddecisions"
)

const (
	reasonHelperBusyOnArrival = "helper_busy_on_arrival"
	reasonOwnWorkArrived      = "own_work_arrived"
)

func decisionRecorderOrNil(store *offloaddecisions.Store) decisionRecorder {
	if store == nil {
		return nil
	}
	return store
}

func (scheduler *scheduler) recordHelperQueueOutcome(lane string, entry *offloadEntry, outcome offloadOutcome) {
	switch {
	case entry.borrowed && outcome == offloadReturned:
		reason := reasonOwnWorkArrived
		if entry.refusedOnArrival {
			reason = reasonHelperBusyOnArrival
		}
		scheduler.record(offloaddecisions.Record{
			Kind:          offloaddecisions.KindHelper,
			Lane:          lane,
			HelperModelID: entry.modelID,
			Outcome:       offloaddecisions.OutcomeBorrowedReturned,
			Reason:        reason,
		})
	case !entry.borrowed && outcome == offloadAdmitted && entry.borrowedAhead > 0:
		scheduler.record(offloaddecisions.Record{
			Kind:          offloaddecisions.KindHelper,
			Lane:          lane,
			OwnerModelID:  entry.modelID,
			Outcome:       offloaddecisions.OutcomeNativeWaitedBehindBorrowed,
			BorrowedAhead: entry.borrowedAhead,
			WaitMS:        time.Since(entry.arrived).Milliseconds(),
		})
	}
}
