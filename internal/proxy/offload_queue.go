package proxy

import (
	"context"
	"errors"
	"sync"
	"time"

	"tensors-router/internal/schedulingcost"
)

var errOffloadReturned = errors.New("offloaded request was returned by the helper node")

type offloadOutcome int

const (
	offloadAdmitted offloadOutcome = iota
	offloadWithdrawn
	offloadReturned
)

// offloadEntry is one request waiting for its turn at the backend. Until it is
// admitted the router still owns it, so it can be withdrawn and sent to a peer
// or handed back to the node that lent it. Once admitted it belongs to the
// backend and runs to completion. requiredContext is 0 for the image lane,
// which has no context window; pinned marks a streaming text request that
// counts toward the node's reported backlog but must never be withdrawn,
// because the router cannot replay a body it never buffered.
type offloadEntry struct {
	groupID         string
	work            schedulingcost.Work
	requiredContext int64
	arrived         time.Time
	borrowed        bool
	pinned          bool
	sequence        uint64
	result          chan offloadOutcome
}

// offloadQueue fronts one backend, one lane at a time (the image lane and the
// text lane each get their own queue). Diffusion and single-slot text backends
// serialize, so the queue exists to hold work the router can still redirect
// rather than to create parallelism: handing everything straight to the
// backend is what makes a backlog impossible to move.
type offloadQueue struct {
	mu       sync.Mutex
	depth    int
	sequence uint64
	pending  []*offloadEntry
	admitted map[*offloadEntry]struct{}
}

func newOffloadQueue(depth int) *offloadQueue {
	if depth < 1 {
		depth = 1
	}
	return &offloadQueue{depth: depth, admitted: map[*offloadEntry]struct{}{}}
}

// Enqueue places a request in line. A borrowed request is refused immediately,
// rather than queued, unless the whole node is idle: nodeIdle is the caller's
// fresh read of every main-line runtime's activity, taken immediately before
// this call rather than from a cached status snapshot, so a native request
// that arrived a moment ago is never raced past. hasNativeWorkLocked still
// catches a native request already sitting in this same queue that nodeIdle,
// a runtime-level signal, cannot see on its own.
func (queue *offloadQueue) Enqueue(groupID string, work schedulingcost.Work, requiredContext int64, borrowed bool, pinned bool, nodeIdle bool, now time.Time) *offloadEntry {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	queue.sequence++
	entry := &offloadEntry{
		groupID:         groupID,
		work:            work,
		requiredContext: requiredContext,
		arrived:         now,
		borrowed:        borrowed,
		pinned:          pinned,
		sequence:        queue.sequence,
		result:          make(chan offloadOutcome, 1),
	}
	if borrowed && (!nodeIdle || queue.hasNativeWorkLocked()) {
		entry.result <- offloadReturned
		return entry
	}
	queue.pending = append(queue.pending, entry)
	if !borrowed {
		queue.returnPendingBorrowedLocked()
	}
	queue.admitLocked()
	return entry
}

// Await blocks until the entry is admitted, withdrawn for offloading, or handed
// back. A cancelled request leaves the queue so it stops holding a slot open.
func (queue *offloadQueue) Await(ctx context.Context, entry *offloadEntry) (offloadOutcome, error) {
	select {
	case outcome := <-entry.result:
		return outcome, nil
	case <-ctx.Done():
		queue.discard(entry)
		return offloadWithdrawn, ctx.Err()
	}
}

// Complete releases the backend slot an admitted entry held and lets the next
// request in. It is idempotent so a deferred call is always safe.
func (queue *offloadQueue) Complete(entry *offloadEntry) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	delete(queue.admitted, entry)
	queue.admitLocked()
}

// WithdrawNewest takes back pending requests so they can be sent to a peer. The
// newest go first: the oldest have waited longest and are nearest the front of
// the queue, so moving them would make them pay a network hop and possibly a
// model load on top of a wait they have already served. Borrowed entries are
// never withdrawn, because this node does not own them; pinned entries are
// never withdrawn either, because the router never buffered a replayable body
// for them.
func (queue *offloadQueue) WithdrawNewest(groupID string, limit int) []*offloadEntry {
	if limit <= 0 {
		return nil
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()

	withdrawn := make([]*offloadEntry, 0, limit)
	for index := len(queue.pending) - 1; index >= 0 && len(withdrawn) < limit; index-- {
		entry := queue.pending[index]
		if entry.borrowed || entry.pinned || entry.groupID != groupID {
			continue
		}
		queue.pending = append(queue.pending[:index], queue.pending[index+1:]...)
		entry.result <- offloadWithdrawn
		withdrawn = append(withdrawn, entry)
	}
	return withdrawn
}

// Requeue puts a returned request back at the head of the line. It has already
// waited once on this node and then again on a peer, so it goes ahead of work
// that has only waited here.
func (queue *offloadQueue) Requeue(groupID string, work schedulingcost.Work, requiredContext int64, now time.Time) *offloadEntry {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	queue.sequence++
	entry := &offloadEntry{
		groupID:         groupID,
		work:            work,
		requiredContext: requiredContext,
		arrived:         now,
		sequence:        queue.sequence,
		result:          make(chan offloadOutcome, 1),
	}
	queue.pending = append([]*offloadEntry{entry}, queue.pending...)
	queue.admitLocked()
	return entry
}

// ReturnBorrowed hands back every borrowed request that has not started. The one
// already running is left alone: the node finishes what it is working on.
func (queue *offloadQueue) ReturnBorrowed() int {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return queue.returnPendingBorrowedLocked()
}

// AcceptingBorrowed reports whether this queue itself may take borrowed work.
// nodeIdle is the caller's own fresh read of the whole node's activity: this
// queue's bookkeeping alone cannot see work in another lane's queue, or work on
// an ungrouped model that never entered any queue at all.
func (queue *offloadQueue) AcceptingBorrowed(nodeIdle bool) bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return nodeIdle && !queue.hasNativeWorkLocked()
}

// BorrowedInFlight counts admitted entries running on this node's behalf of a
// peer. A request only reaches admitted after it has already been counted in a
// runtime's own user count, so this is what a node-wide busy signal must
// subtract to avoid a helper looking busy from the very job it was lent.
func (queue *offloadQueue) BorrowedInFlight() int {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	count := 0
	for entry := range queue.admitted {
		if entry.borrowed {
			count++
		}
	}
	return count
}

// offloadGroupStats separates what the node can still hand over from everything
// it has left to do. Pending work is withdrawable; the job already admitted to
// the backend is not, but it still sits ahead of that queue and so still counts
// toward how long the node will take to finish. PendingContext is the summed
// required context of the withdrawable entries, 0 for the image lane.
type offloadGroupStats struct {
	GroupID        string              `json:"group_id"`
	PendingCount   int64               `json:"pending_count"`
	PendingWork    schedulingcost.Work `json:"pending_work"`
	PendingContext int64               `json:"pending_context,omitempty"`
	BacklogCount   int64               `json:"backlog_count"`
	BacklogWork    schedulingcost.Work `json:"backlog_work"`
}

func (queue *offloadQueue) Stats() []offloadGroupStats {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	byGroup := map[string]*offloadGroupStats{}
	var order []string
	statsFor := func(groupID string) *offloadGroupStats {
		stats, seen := byGroup[groupID]
		if !seen {
			stats = &offloadGroupStats{GroupID: groupID}
			byGroup[groupID] = stats
			order = append(order, groupID)
		}
		return stats
	}
	for _, entry := range queue.pending {
		if entry.borrowed {
			continue
		}
		stats := statsFor(entry.groupID)
		stats.PendingCount++
		if sum, ok := stats.PendingWork.Add(entry.work); ok {
			stats.PendingWork = sum
		}
		stats.PendingContext += entry.requiredContext
		stats.BacklogCount++
		if sum, ok := stats.BacklogWork.Add(entry.work); ok {
			stats.BacklogWork = sum
		}
	}
	for entry := range queue.admitted {
		if entry.borrowed {
			continue
		}
		stats := statsFor(entry.groupID)
		stats.BacklogCount++
		if sum, ok := stats.BacklogWork.Add(entry.work); ok {
			stats.BacklogWork = sum
		}
	}
	result := make([]offloadGroupStats, 0, len(order))
	for _, groupID := range order {
		result = append(result, *byGroup[groupID])
	}
	return result
}

func (queue *offloadQueue) discard(entry *offloadEntry) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for index, pending := range queue.pending {
		if pending == entry {
			queue.pending = append(queue.pending[:index], queue.pending[index+1:]...)
			break
		}
	}
	delete(queue.admitted, entry)
	queue.admitLocked()
}

func (queue *offloadQueue) admitLocked() {
	for len(queue.admitted) < queue.depth && len(queue.pending) > 0 {
		entry := queue.pending[0]
		queue.pending = queue.pending[1:]
		queue.admitted[entry] = struct{}{}
		entry.result <- offloadAdmitted
	}
}

func (queue *offloadQueue) returnPendingBorrowedLocked() int {
	kept := queue.pending[:0]
	returned := 0
	for _, entry := range queue.pending {
		if entry.borrowed {
			entry.result <- offloadReturned
			returned++
			continue
		}
		kept = append(kept, entry)
	}
	queue.pending = kept
	return returned
}

// hasNativeWorkLocked reports whether this queue holds any work of its own,
// pending or admitted. It answers "is there a native entry already in this
// queue", not "is the runtime busy" — idleForBorrowedWork answers that,
// including native work this queue never saw because it belongs to an
// ungrouped model or the other lane's queue.
func (queue *offloadQueue) hasNativeWorkLocked() bool {
	for entry := range queue.admitted {
		if !entry.borrowed {
			return true
		}
	}
	for _, entry := range queue.pending {
		if !entry.borrowed {
			return true
		}
	}
	return false
}
