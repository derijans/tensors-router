package proxy

import (
	"testing"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/offloadsettings"
)

func TestHoldConsultsOnlyForThePipeSlot(t *testing.T) {
	queue := newOffloadQueue(2)
	var consulted []ownerPipeline
	queue.SetAdmissionHold(func(_ *offloadEntry, pipeline ownerPipeline) bool {
		consulted = append(consulted, pipeline)
		return true
	})

	generating := enqueueNativeOnIdleNode(queue, "group", 10)
	waiting := enqueueNativeOnIdleNode(queue, "group", 10)

	mustBeAdmitted(t, generating, "generating")
	mustBeWaiting(t, waiting, "waiting")
	if len(consulted) != 1 || consulted[0].inFlight != 1 {
		t.Fatalf("hold consulted with %+v, want once, for the pipe slot behind one generating request", consulted)
	}

	queue.Complete(generating)
	mustBeAdmitted(t, waiting, "waiting once the owner would otherwise idle")
}

func TestSnapshotCallsARequestHeldOnlyWhileTheHoldKeepsItBack(t *testing.T) {
	queue := newOffloadQueue(1)
	queue.SetAdmissionHold(func(*offloadEntry, ownerPipeline) bool { return true })
	enqueueNativeOnIdleNode(queue, "group", 10)
	enqueueNativeOnIdleNode(queue, "group", 10)

	if snapshot := queue.HeldSnapshot(); len(snapshot) != 1 || snapshot[0].holding {
		t.Fatalf("snapshot = %+v, want one request merely queued behind a full pipe", snapshot)
	}

	queue.SetDepth(2)

	if snapshot := queue.HeldSnapshot(); len(snapshot) != 1 || !snapshot[0].holding {
		t.Fatalf("snapshot = %+v, want the request the hold is keeping back reported as held", snapshot)
	}
}

func TestRequeuedRequestKeepsTheTimeItFirstArrived(t *testing.T) {
	queue := newOffloadQueue(1)
	firstArrived := time.Now().Add(-90 * time.Second)

	returned := queue.Requeue("group", enqueueNativeOnIdleNode(newOffloadQueue(2), "group", 10).work, 0, firstArrived)

	if !returned.arrived.Equal(firstArrived) {
		t.Fatalf("arrived = %s, want the original %s so the wait shown to the operator does not restart", returned.arrived, firstArrived)
	}
}

func TestHoldIsNeverAskedAboutPinnedOrReturnedRequests(t *testing.T) {
	queue := newOffloadQueue(2)
	queue.SetAdmissionHold(func(*offloadEntry, ownerPipeline) bool { return true })
	generating := enqueueNativeOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, generating, "generating")

	pinned := queue.Enqueue(queuedRequest{modelID: "group", origin: nativeRequest, withdrawal: pinnedToThisNode}, nodeActivity(true), time.Now())
	mustBeAdmitted(t, pinned, "pinned")

	queue.Complete(pinned)
	returned := queue.Requeue("group", pinned.work, 0, time.Now())
	mustBeAdmitted(t, returned, "returned by the helper")
}

func TestPipelineCountsGenerationFromTheLastCompletion(t *testing.T) {
	queue := newOffloadQueue(2)
	var seen ownerPipeline
	queue.SetAdmissionHold(func(_ *offloadEntry, pipeline ownerPipeline) bool {
		seen = pipeline
		return false
	})
	first := enqueueNativeOnIdleNode(queue, "group", 10)
	second := enqueueNativeOnIdleNode(queue, "group", 10)
	enqueueNativeOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, first, "first")
	mustBeAdmitted(t, second, "second")

	beforeCompletion := time.Now()
	queue.Complete(first)

	if seen.inFlight != 1 || seen.generatingSince.Before(beforeCompletion) {
		t.Fatalf("pipeline = %+v, want one in flight generating since the completion at %s, not since it was admitted", seen, beforeCompletion)
	}
}

func TestHoldIsNotConsultedBehindAnotherModel(t *testing.T) {
	queue := newOffloadQueue(2)
	consulted := false
	queue.SetAdmissionHold(func(*offloadEntry, ownerPipeline) bool {
		consulted = true
		return true
	})
	other := enqueueNativeOnIdleNode(queue, "other", 10)
	next := enqueueNativeOnIdleNode(queue, "group", 10)

	mustBeAdmitted(t, other, "other model")
	mustBeAdmitted(t, next, "next")
	if consulted {
		t.Fatal("hold consulted while the backend runs another model the lease knows nothing about")
	}
}

func TestPredictedFinishesMatchTheObservedLendingBurst(t *testing.T) {
	burst := time.Date(2026, 9, 28, 0, 57, 19, 0, time.UTC)
	ownerCompletedFirst := burst.Add(29 * time.Second)
	lease := observedBurstLease()
	lent := []lentRequest{{lentAt: burst.Add(5 * time.Second)}}

	ownerFinish := ownerPipeline{inFlight: 1, generatingSince: ownerCompletedFirst}.nextJobFinish(ownerCompletedFirst, lease.OwnerJobMS)
	helperFinish := lease.nextJobFinishAfter(lent, ownerCompletedFirst)

	if want := ownerCompletedFirst.Add(58 * time.Second); !ownerFinish.Equal(want) {
		t.Fatalf("owner finish = %s, want %s: the generating job plus this one", ownerFinish, want)
	}
	if want := ownerCompletedFirst.Add(13 * time.Second); !helperFinish.Equal(want) {
		t.Fatalf("helper finish = %s, want %s: helper already free, one warm job", helperFinish, want)
	}
}

func TestHelperFinishQueuesBehindEveryLentRequest(t *testing.T) {
	now := time.Date(2026, 9, 28, 1, 0, 0, 0, time.UTC)
	lease := observedBurstLease()
	lent := []lentRequest{{lentAt: now}, {lentAt: now}}

	helperFinish := lease.nextJobFinishAfter(lent, now)

	if want := now.Add((6 + 3*13) * time.Second); !helperFinish.Equal(want) {
		t.Fatalf("helper finish = %s, want %s: load, both lent jobs, then this one", helperFinish, want)
	}
}

func observedBurstLease() offloadLease {
	lease := imageLeaseWithSlots(1)
	lease.OwnerJobMS = 29000
	lease.HelperSwitchMS = 6000
	lease.HelperServiceMS = 13000
	return lease
}

func schedulerWithLentBurst(t *testing.T, lease offloadLease) (*scheduler, *recordedDecisions, []*offloadEntry, []*offloadEntry) {
	t.Helper()
	scheduler, recorded := schedulerRecordingDecisions(t)
	entries := holdNativeImageRequests(scheduler, 4)
	mustBeAdmitted(t, entries[0], "first")
	mustBeAdmitted(t, entries[1], "second")
	scheduler.acceptOffloadLease(lease, queueEventEnqueued)
	lent := withdrawnEntries(entries)
	if len(lent) != 1 {
		t.Fatalf("lent %d, want the lease's single slot filled", len(lent))
	}
	return scheduler, recorded, entries, lent
}

func TestOwnerKeepsTheWaitingRequestForAFasterBusyHelper(t *testing.T) {
	scheduler, recorded, entries, _ := schedulerWithLentBurst(t, observedBurstLease())

	scheduler.completeImageQueueEntry(lentModelID, entries[0])

	mustBeWaiting(t, entries[2], "waiting request")
	held := recorded.withOutcome(offloaddecisions.OutcomeHeldForHelper)
	if len(held) != 1 || held[0].Reason != reasonHelperFinishesSooner || held[0].OwnerJobMS != 29000 {
		t.Fatalf("held decisions = %+v, want one explaining the helper finishes sooner", held)
	}
}

func TestHeldRequestIsLentOnTheMastersNextAnswer(t *testing.T) {
	scheduler, _, entries, lent := schedulerWithLentBurst(t, observedBurstLease())
	scheduler.completeImageQueueEntry(lentModelID, entries[0])

	scheduler.finishOffload(cluster.RouteLaneImage, lentModelID, lent[0], false)
	mustBeWaiting(t, entries[2], "held request before the master answers")
	scheduler.acceptOffloadLease(observedBurstLease(), queueEventBorrowedCompleted)

	if next := withdrawnEntries(entries); len(next) != 1 || next[0] != entries[2] {
		t.Fatalf("withdrawn %v, want the held request lent to the freed helper", next)
	}
}

func TestHeldRequestGoesToTheOwnerWhenTheMasterClearsTheLease(t *testing.T) {
	scheduler, _, entries, _ := schedulerWithLentBurst(t, observedBurstLease())
	scheduler.completeImageQueueEntry(lentModelID, entries[0])
	mustBeWaiting(t, entries[2], "held request")

	scheduler.applyDecidedLease(queueEvent{Lane: cluster.RouteLaneImage, ModelID: lentModelID, Trigger: queueEventCompleted}, offloadLease{}, false)

	mustBeAdmitted(t, entries[2], "held request after the lease was cleared")
}

func TestHeldRequestGoesToTheOwnerBeforeItWouldIdle(t *testing.T) {
	scheduler, _, entries, _ := schedulerWithLentBurst(t, observedBurstLease())
	scheduler.completeImageQueueEntry(lentModelID, entries[0])
	mustBeWaiting(t, entries[2], "held request")

	scheduler.completeImageQueueEntry(lentModelID, entries[1])

	mustBeAdmitted(t, entries[2], "held request once the owner has nothing else to generate")
}

func TestOwnerTakesTheWaitingRequestWhenTheHelperIsSlower(t *testing.T) {
	lease := observedBurstLease()
	lease.HelperServiceMS = 60000
	scheduler, recorded, entries, _ := schedulerWithLentBurst(t, lease)

	scheduler.completeImageQueueEntry(lentModelID, entries[0])

	mustBeAdmitted(t, entries[2], "waiting request")
	if held := recorded.withOutcome(offloaddecisions.OutcomeHeldForHelper); len(held) != 0 {
		t.Fatalf("held decisions = %+v, want none for a slower helper", held)
	}
}

func TestOwnerTakesTheWaitingRequestUnderAProbeLease(t *testing.T) {
	lease := imageLeaseWithSlots(1)
	lease.Probe = true
	scheduler, _, entries, _ := schedulerWithLentBurst(t, lease)

	scheduler.completeImageQueueEntry(lentModelID, entries[0])

	mustBeAdmitted(t, entries[2], "waiting request under a lease with no measured speeds")
}

func TestOwnerTakesTheWaitingRequestWhenHoldingIsSwitchedOff(t *testing.T) {
	scheduler, _, entries, _ := schedulerWithLentBurst(t, observedBurstLease())
	tuneScheduler(scheduler, func(settings *offloadsettings.Settings) { settings.HoldForFasterHelper = false })

	scheduler.completeImageQueueEntry(lentModelID, entries[0])

	mustBeAdmitted(t, entries[2], "waiting request with holding switched off")
}

func TestSwitchingHoldingOffReleasesAHeldRequest(t *testing.T) {
	scheduler, _, entries, _ := schedulerWithLentBurst(t, observedBurstLease())
	scheduler.completeImageQueueEntry(lentModelID, entries[0])
	mustBeWaiting(t, entries[2], "held request")

	tuneScheduler(scheduler, func(settings *offloadsettings.Settings) { settings.HoldForFasterHelper = false })

	mustBeAdmitted(t, entries[2], "held request once holding is switched off")
}
