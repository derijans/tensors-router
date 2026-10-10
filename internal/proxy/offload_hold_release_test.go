package proxy

import (
	"testing"
	"time"
)

func backdateLentRequest(scheduler *scheduler, entry *offloadEntry, ago time.Duration) {
	scheduler.lent.mu.Lock()
	defer scheduler.lent.mu.Unlock()
	request := scheduler.lent.byEntry[entry]
	request.lentAt = request.lentAt.Add(-ago)
	scheduler.lent.byEntry[entry] = request
}

func TestHeldRequestIsReleasedOnceTheHelperOverranItsPredictedFinish(t *testing.T) {
	scheduler, _, entries, lent := schedulerWithLentBurst(t, observedBurstLease())
	scheduler.completeImageQueueEntry(lentModelID, entries[0])
	mustBeWaiting(t, entries[2], "held request while the helper is on schedule")

	backdateLentRequest(scheduler, lent[0], 10*time.Minute)
	scheduler.imageQueue.Readmit()

	mustBeAdmitted(t, entries[2], "held request after the helper took far longer than predicted")
}

func TestHeldRequestIsReleasedByTheClockWhenNothingElseHappens(t *testing.T) {
	lease := observedBurstLease()
	lease.HelperSwitchMS = 0
	lease.HelperServiceMS = 40
	scheduler, _, entries, _ := schedulerWithLentBurst(t, lease)

	scheduler.completeImageQueueEntry(lentModelID, entries[0])

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, resolved := outcomeNow(t, entries[2]); resolved {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("held request was never released although the helper overran and no queue event followed")
}

func TestLeaseGivesUpOnALentRequestOnlyWhenItKnowsHowLongTheHelperNeeds(t *testing.T) {
	if got, want := observedBurstLease().giveUpAfter(), 3*19*time.Second+lentGiveUpFloor; got != want {
		t.Fatalf("give up after = %v, want %v", got, want)
	}
	probe := imageLeaseWithSlots(1)
	probe.Probe = true
	if got := probe.giveUpAfter(); got != 0 {
		t.Fatalf("give up after = %v for a lease with no measured speeds, want no deadline", got)
	}
}
