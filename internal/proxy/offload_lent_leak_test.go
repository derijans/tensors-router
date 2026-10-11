package proxy

import (
	"context"
	"runtime"
	"sync"
	"testing"

	"tensors-router/internal/cluster"
	"tensors-router/internal/schedulingcost"
)

func TestRequestReturnedByTheHelperIsNeverLentAgain(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	entries := holdNativeImageRequests(scheduler, 3)
	scheduler.acceptOffloadLease(imageLeaseWithSlots(1), queueEventEnqueued)
	lent := withdrawnEntries(entries)
	if len(lent) != 1 {
		t.Fatalf("lent %d requests, want 1", len(lent))
	}
	scheduler.finishOffload(cluster.RouteLaneImage, lentModelID, lent[0], true)
	requeued := scheduler.imageQueue.Requeue(lentModelID, lent[0].work, 0, lent[0].arrived)

	scheduler.acceptOffloadLease(imageLeaseWithSlots(1), queueEventBorrowedCompleted)

	if again := withdrawnEntries([]*offloadEntry{requeued}); len(again) != 0 {
		t.Fatalf("the request the helper returned was lent again, want it served by its owner")
	}
	if count := scheduler.lent.Count(cluster.RouteLaneImage, lentModelID); count != 0 {
		t.Fatalf("lent out %d, want 0 after the only lent request came back", count)
	}
}

func TestClientCancellingWhileBeingLentDoesNotLeakTheLentSlot(t *testing.T) {
	scheduler, _ := schedulerRecordingDecisions(t)
	holdNativeImageRequests(scheduler, 2)
	for attempt := range 2000 {
		ctx, cancel := context.WithCancel(context.Background())
		var enteredErr error
		var admission queueAdmission
		var waiting sync.WaitGroup
		waiting.Add(1)
		go func() {
			defer waiting.Done()
			admission, enteredErr = scheduler.enterImageQueue(ctx, lentModelID, schedulingcost.ImageWork(testJobWork), false)
		}()
		waitForPendingNative(t, scheduler)

		var racing sync.WaitGroup
		racing.Add(2)
		go func() {
			defer racing.Done()
			for range attempt % 40 {
				runtime.Gosched()
			}
			cancel()
		}()
		go func() {
			defer racing.Done()
			scheduler.acceptOffloadLease(imageLeaseWithSlots(1), queueEventEnqueued)
		}()
		racing.Wait()
		waiting.Wait()

		if enteredErr == nil && admission.outcome == offloadWithdrawn {
			scheduler.finishOffload(cluster.RouteLaneImage, lentModelID, admission.entry, false)
		}
		if count := scheduler.lent.Count(cluster.RouteLaneImage, lentModelID); count != 0 {
			t.Fatalf("attempt %d: lent out %d after the request ended (err=%v), want 0", attempt, count, enteredErr)
		}
	}
}
