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

type requestOrigin int

const (
	nativeRequest requestOrigin = iota
	borrowedFromPeer
)

type withdrawability int

const (
	withdrawable withdrawability = iota
	pinnedToThisNode
)

type queuedRequest struct {
	groupID         string
	work            schedulingcost.Work
	requiredContext int64
	origin          requestOrigin
	withdrawal      withdrawability
}

type nodeActivity bool

func (service *Service) nodeActivity() nodeActivity {
	return nodeActivity(service.idleForBorrowedWork())
}

func (activity nodeActivity) idle() bool {
	return bool(activity)
}

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

func (queue *offloadQueue) Enqueue(request queuedRequest, node nodeActivity, now time.Time) *offloadEntry {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	queue.sequence++
	borrowed := request.origin == borrowedFromPeer
	entry := &offloadEntry{
		groupID:         request.groupID,
		work:            request.work,
		requiredContext: request.requiredContext,
		arrived:         now,
		borrowed:        borrowed,
		pinned:          request.withdrawal == pinnedToThisNode,
		sequence:        queue.sequence,
		result:          make(chan offloadOutcome, 1),
	}
	if borrowed && (!node.idle() || queue.holdsOwnWorkLocked()) {
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

func (queue *offloadQueue) Await(ctx context.Context, entry *offloadEntry) (offloadOutcome, error) {
	select {
	case outcome := <-entry.result:
		return outcome, nil
	case <-ctx.Done():
		queue.discard(entry)
		return offloadWithdrawn, ctx.Err()
	}
}

func (queue *offloadQueue) Complete(entry *offloadEntry) {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	delete(queue.admitted, entry)
	queue.admitLocked()
}

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

func (queue *offloadQueue) ReturnBorrowed() int {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return queue.returnPendingBorrowedLocked()
}

func (queue *offloadQueue) AcceptingBorrowed(node nodeActivity) bool {
	queue.mu.Lock()
	defer queue.mu.Unlock()
	return node.idle() && !queue.holdsOwnWorkLocked()
}

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

func (queue *offloadQueue) holdsOwnWorkLocked() bool {
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
