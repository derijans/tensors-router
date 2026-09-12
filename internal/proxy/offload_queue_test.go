package proxy

import (
	"context"
	"testing"
	"time"

	"tensors-router/internal/schedulingcost"
)

func outcomeNow(t *testing.T, entry *offloadEntry) (offloadOutcome, bool) {
	t.Helper()
	select {
	case outcome := <-entry.result:
		return outcome, true
	default:
		return 0, false
	}
}

func mustBeAdmitted(t *testing.T, entry *offloadEntry, label string) {
	t.Helper()
	outcome, ok := outcomeNow(t, entry)
	if !ok {
		t.Fatalf("%s is still waiting, want admitted", label)
	}
	if outcome != offloadAdmitted {
		t.Fatalf("%s outcome = %v, want admitted", label, outcome)
	}
}

func mustBeWaiting(t *testing.T, entry *offloadEntry, label string) {
	t.Helper()
	if outcome, ok := outcomeNow(t, entry); ok {
		t.Fatalf("%s resolved to %v, want still waiting", label, outcome)
	}
}

func enqueueNativeOnIdleNode(queue *offloadQueue, groupID string, work float64) *offloadEntry {
	return queue.Enqueue(queuedRequest{groupID: groupID, work: schedulingcost.ImageWork(work), origin: nativeRequest}, nodeActivity(true), time.Now())
}

func enqueueBorrowedOnIdleNode(queue *offloadQueue, groupID string, work float64) *offloadEntry {
	return queue.Enqueue(queuedRequest{groupID: groupID, work: schedulingcost.ImageWork(work), origin: borrowedFromPeer}, nodeActivity(true), time.Now())
}

// The whole point of the queue is that the backend keeps exactly one job running
// and one queued behind it, so it never idles between jobs while the router still
// holds everything else and can move it.
func TestQueueAdmitsUpToDepthAndHoldsTheRest(t *testing.T) {
	queue := newOffloadQueue(2)
	first := enqueueNativeOnIdleNode(queue, "group", 10)
	second := enqueueNativeOnIdleNode(queue, "group", 10)
	third := enqueueNativeOnIdleNode(queue, "group", 10)

	mustBeAdmitted(t, first, "first")
	mustBeAdmitted(t, second, "second")
	mustBeWaiting(t, third, "third")
}

func TestQueueAdmitsTheNextEntryOnCompletion(t *testing.T) {
	queue := newOffloadQueue(1)
	first := enqueueNativeOnIdleNode(queue, "group", 10)
	second := enqueueNativeOnIdleNode(queue, "group", 10)
	third := enqueueNativeOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, first, "first")
	mustBeWaiting(t, second, "second")

	queue.Complete(first)
	mustBeAdmitted(t, second, "second")
	mustBeWaiting(t, third, "third")

	queue.Complete(second)
	mustBeAdmitted(t, third, "third")
}

// Withdrawing the oldest would make a request that has nearly reached the front
// pay a network hop and possibly a model load on top of the wait it already
// served, so offloading takes from the back.
func TestWithdrawTakesNewestFirstAndOnlyPendingEntries(t *testing.T) {
	queue := newOffloadQueue(1)
	running := enqueueNativeOnIdleNode(queue, "group", 10)
	oldest := enqueueNativeOnIdleNode(queue, "group", 20)
	middle := enqueueNativeOnIdleNode(queue, "group", 30)
	newest := enqueueNativeOnIdleNode(queue, "group", 40)
	mustBeAdmitted(t, running, "running")

	withdrawn := queue.WithdrawNewest("group", 2)
	if len(withdrawn) != 2 {
		t.Fatalf("withdrew %d, want 2", len(withdrawn))
	}
	if withdrawn[0] != newest || withdrawn[1] != middle {
		t.Fatal("withdrew entries out of newest-first order")
	}
	for _, entry := range withdrawn {
		outcome, ok := outcomeNow(t, entry)
		if !ok || outcome != offloadWithdrawn {
			t.Fatalf("withdrawn entry outcome = %v ok=%t", outcome, ok)
		}
	}
	mustBeWaiting(t, oldest, "oldest")
}

func TestWithdrawNeverTakesTheRunningRequest(t *testing.T) {
	queue := newOffloadQueue(1)
	running := enqueueNativeOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, running, "running")

	if withdrawn := queue.WithdrawNewest("group", 5); len(withdrawn) != 0 {
		t.Fatalf("withdrew %d admitted entries, want 0", len(withdrawn))
	}
}

func TestWithdrawIgnoresOtherGroupsAndBorrowedWork(t *testing.T) {
	queue := newOffloadQueue(1)
	running := enqueueNativeOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, running, "running")
	other := enqueueNativeOnIdleNode(queue, "other-group", 10)
	mine := enqueueNativeOnIdleNode(queue, "group", 10)

	withdrawn := queue.WithdrawNewest("group", 5)
	if len(withdrawn) != 1 || withdrawn[0] != mine {
		t.Fatalf("withdrew %d entries, want only the one in the named group", len(withdrawn))
	}
	mustBeWaiting(t, other, "other group entry")
}

// A helper lends idle time. If it has work of its own there is no idle time to
// lend, so borrowed work is refused at the door rather than queued behind it.
func TestBorrowedWorkIsRefusedWhileTheNodeHasItsOwn(t *testing.T) {
	queue := newOffloadQueue(2)
	native := enqueueNativeOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, native, "native")

	borrowed := enqueueBorrowedOnIdleNode(queue, "group", 10)
	outcome, ok := outcomeNow(t, borrowed)
	if !ok || outcome != offloadReturned {
		t.Fatalf("borrowed outcome = %v ok=%t, want returned", outcome, ok)
	}
	if queue.AcceptingBorrowed(nodeActivity(true)) {
		t.Fatal("queue reports it is accepting borrowed work while running its own")
	}
}

// The two halves of the rule: finish the borrowed job already running, hand back
// every borrowed job that has not started.
func TestNativeWorkReturnsPendingBorrowedButNotTheRunningOne(t *testing.T) {
	queue := newOffloadQueue(1)
	running := enqueueBorrowedOnIdleNode(queue, "group", 10)
	waiting := enqueueBorrowedOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, running, "running borrowed")
	mustBeWaiting(t, waiting, "waiting borrowed")

	native := enqueueNativeOnIdleNode(queue, "group", 10)

	outcome, ok := outcomeNow(t, waiting)
	if !ok || outcome != offloadReturned {
		t.Fatalf("pending borrowed outcome = %v ok=%t, want returned", outcome, ok)
	}
	mustBeWaiting(t, native, "native")

	queue.Complete(running)
	mustBeAdmitted(t, native, "native")
}

func TestQueueAcceptsBorrowedWorkAgainOnceItsOwnQueueDrains(t *testing.T) {
	queue := newOffloadQueue(1)
	native := enqueueNativeOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, native, "native")
	if queue.AcceptingBorrowed(nodeActivity(true)) {
		t.Fatal("accepting borrowed work while its own is running")
	}

	queue.Complete(native)
	if !queue.AcceptingBorrowed(nodeActivity(true)) {
		t.Fatal("still refusing borrowed work after its own queue drained")
	}
	borrowed := enqueueBorrowedOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, borrowed, "borrowed")
}

func TestQueueRefusesBorrowedWorkWhileTheNodeIsBusyElsewhere(t *testing.T) {
	queue := newOffloadQueue(2)
	borrowed := queue.Enqueue(queuedRequest{groupID: "group", work: schedulingcost.ImageWork(10), origin: borrowedFromPeer}, nodeActivity(false), time.Now())
	outcome, ok := outcomeNow(t, borrowed)
	if !ok || outcome != offloadReturned {
		t.Fatalf("borrowed outcome = %v ok=%t, want returned when the node is not idle", outcome, ok)
	}
	if queue.AcceptingBorrowed(nodeActivity(false)) {
		t.Fatal("queue reports accepting borrowed work while the node is not idle")
	}
}

// A returned request has already waited here and then again on a peer, so it goes
// ahead of work that has only waited here once.
func TestRequeuePlacesReturnedWorkAtTheHead(t *testing.T) {
	queue := newOffloadQueue(1)
	running := enqueueNativeOnIdleNode(queue, "group", 10)
	waiting := enqueueNativeOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, running, "running")

	returned := queue.Requeue("group", schedulingcost.ImageWork(10), 0, time.Now())
	mustBeWaiting(t, returned, "returned")
	mustBeWaiting(t, waiting, "waiting")

	queue.Complete(running)
	mustBeAdmitted(t, returned, "returned")
	mustBeWaiting(t, waiting, "waiting")
}

func TestCancelledRequestFreesItsSlot(t *testing.T) {
	queue := newOffloadQueue(1)
	running := enqueueNativeOnIdleNode(queue, "group", 10)
	mustBeAdmitted(t, running, "running")
	waiting := enqueueNativeOnIdleNode(queue, "group", 10)
	behind := enqueueNativeOnIdleNode(queue, "group", 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := queue.Await(ctx, waiting); err == nil {
		t.Fatal("await of a cancelled request returned no error")
	}

	queue.Complete(running)
	mustBeAdmitted(t, behind, "entry behind the cancelled one")
}

func TestStatsReportPendingOwnWorkPerGroup(t *testing.T) {
	queue := newOffloadQueue(1)
	running := enqueueNativeOnIdleNode(queue, "group", 5)
	mustBeAdmitted(t, running, "running")
	enqueueNativeOnIdleNode(queue, "group", 10)
	enqueueNativeOnIdleNode(queue, "group", 20)
	enqueueNativeOnIdleNode(queue, "other", 40)

	stats := queue.Stats()
	byGroup := map[string]offloadGroupStats{}
	for _, item := range stats {
		byGroup[item.GroupID] = item
	}
	if got := byGroup["group"]; got.PendingCount != 2 || got.PendingWork.Term(0) != 30 {
		t.Fatalf("group stats = %+v, want 2 pending totalling 30", got)
	}
	if got := byGroup["other"]; got.PendingCount != 1 || got.PendingWork.Term(0) != 40 {
		t.Fatalf("other group stats = %+v, want 1 pending totalling 40", got)
	}
}

// Borrowed work is not this node's backlog, so advertising it would invite the
// master to offload work that is already being offloaded.
func TestStatsExcludeBorrowedWork(t *testing.T) {
	queue := newOffloadQueue(1)
	running := enqueueBorrowedOnIdleNode(queue, "group", 5)
	mustBeAdmitted(t, running, "running borrowed")
	enqueueBorrowedOnIdleNode(queue, "group", 10)

	if stats := queue.Stats(); len(stats) != 0 {
		t.Fatalf("stats = %+v, want none for borrowed work", stats)
	}
}

func TestAwaitReturnsTheDeliveredOutcome(t *testing.T) {
	queue := newOffloadQueue(1)
	entry := enqueueNativeOnIdleNode(queue, "group", 10)
	outcome, err := queue.Await(context.Background(), entry)
	if err != nil || outcome != offloadAdmitted {
		t.Fatalf("await = %v err=%v, want admitted", outcome, err)
	}
}

// The running job cannot be handed to a peer, but it is still ahead of the queue,
// so it counts toward how long this node will take even though it is not
// withdrawable. Conflating the two would make an owner look emptier than it is.
func TestStatsSeparateWithdrawableWorkFromTotalBacklog(t *testing.T) {
	queue := newOffloadQueue(1)
	running := enqueueNativeOnIdleNode(queue, "group", 5)
	mustBeAdmitted(t, running, "running")
	enqueueNativeOnIdleNode(queue, "group", 10)
	enqueueNativeOnIdleNode(queue, "group", 20)

	stats := queue.Stats()
	if len(stats) != 1 {
		t.Fatalf("stats = %+v, want one group", stats)
	}
	if stats[0].PendingCount != 2 || stats[0].PendingWork.Term(0) != 30 {
		t.Fatalf("withdrawable = %d/%v, want 2/30", stats[0].PendingCount, stats[0].PendingWork)
	}
	if stats[0].BacklogCount != 3 || stats[0].BacklogWork.Term(0) != 35 {
		t.Fatalf("backlog = %d/%v, want 3/35 including the running job", stats[0].BacklogCount, stats[0].BacklogWork)
	}
}

func TestStatsCountABacklogThatIsOnlyRunningWork(t *testing.T) {
	queue := newOffloadQueue(2)
	running := enqueueNativeOnIdleNode(queue, "group", 7)
	mustBeAdmitted(t, running, "running")

	stats := queue.Stats()
	if len(stats) != 1 || stats[0].PendingCount != 0 || stats[0].BacklogCount != 1 || stats[0].BacklogWork.Term(0) != 7 {
		t.Fatalf("stats = %+v, want nothing withdrawable but a backlog of one", stats)
	}
}

func TestTextQueueRefusesBorrowedWorkWhileNativeWorkIsPending(t *testing.T) {
	queue := newOffloadQueue(1)
	native := queue.Enqueue(queuedRequest{groupID: "group", work: schedulingcost.TextWork(200, 50), requiredContext: 4096, origin: nativeRequest}, nodeActivity(true), time.Now())
	mustBeAdmitted(t, native, "native")

	borrowed := queue.Enqueue(queuedRequest{groupID: "group", work: schedulingcost.TextWork(200, 50), requiredContext: 4096, origin: borrowedFromPeer}, nodeActivity(true), time.Now())
	outcome, ok := outcomeNow(t, borrowed)
	if !ok || outcome != offloadReturned {
		t.Fatalf("borrowed outcome = %v ok=%t, want returned", outcome, ok)
	}
}

func TestPinnedEntryCountsTowardBacklogButIsNeverWithdrawn(t *testing.T) {
	queue := newOffloadQueue(2)
	running := queue.Enqueue(queuedRequest{groupID: "group", work: schedulingcost.TextWork(100, 20), requiredContext: 2048, origin: nativeRequest}, nodeActivity(true), time.Now())
	mustBeAdmitted(t, running, "running")
	pinned := queue.Enqueue(queuedRequest{groupID: "group", work: schedulingcost.TextWork(100, 20), requiredContext: 2048, origin: nativeRequest, withdrawal: pinnedToThisNode}, nodeActivity(true), time.Now())
	mustBeAdmitted(t, pinned, "pinned")

	if withdrawn := queue.WithdrawNewest("group", 5); len(withdrawn) != 0 {
		t.Fatalf("withdrew %d entries, want none when every entry is admitted or pinned", len(withdrawn))
	}

	stats := queue.Stats()
	if len(stats) != 1 || stats[0].BacklogCount != 2 {
		t.Fatalf("stats = %+v, want the pinned entry counted in the backlog", stats)
	}
}

func TestTextQueueStatsCarryBothWorkTermsAndContext(t *testing.T) {
	queue := newOffloadQueue(1)
	running := queue.Enqueue(queuedRequest{groupID: "group", work: schedulingcost.TextWork(100, 20), requiredContext: 2048, origin: nativeRequest}, nodeActivity(true), time.Now())
	mustBeAdmitted(t, running, "running")
	queue.Enqueue(queuedRequest{groupID: "group", work: schedulingcost.TextWork(200, 40), requiredContext: 4096, origin: nativeRequest}, nodeActivity(true), time.Now())

	stats := queue.Stats()
	if len(stats) != 1 {
		t.Fatalf("stats = %+v, want one group", stats)
	}
	if stats[0].PendingWork.Term(0) != 200 || stats[0].PendingWork.Term(1) != 40 {
		t.Fatalf("pending work = %+v, want (200, 40)", stats[0].PendingWork)
	}
	if stats[0].PendingContext != 4096 {
		t.Fatalf("pending context = %d, want 4096", stats[0].PendingContext)
	}
}
