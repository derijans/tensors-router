package proxy

type switchWaiter struct {
	satisfiedBy func(*activeConfigState) bool
}

type switchQueue struct {
	lastTicket   uint64
	windowTicket uint64
	waiters      map[uint64]switchWaiter
}

func neverSatisfied(*activeConfigState) bool {
	return false
}

func (state *activeConfigState) takeTicketLocked() uint64 {
	state.queue.lastTicket++
	return state.queue.lastTicket
}

func (state *activeConfigState) joinQueueLocked(ticket uint64, satisfiedBy func(*activeConfigState) bool) {
	if state.queue.waiters == nil {
		state.queue.waiters = map[uint64]switchWaiter{}
	}
	state.queue.waiters[ticket] = switchWaiter{satisfiedBy: satisfiedBy}
}

func (state *activeConfigState) leaveQueueLocked(ticket uint64) {
	if _, waiting := state.queue.waiters[ticket]; !waiting {
		return
	}
	delete(state.queue.waiters, ticket)
	notifyActiveConfigLocked(state)
}

func (state *activeConfigState) queuedWaitersLocked() int {
	return len(state.queue.waiters)
}

func (state *activeConfigState) openLeaseWindowLocked() {
	state.queue.windowTicket = state.queue.lastTicket + 1
}

func (state *activeConfigState) admitsLeaseLocked(ticket uint64) bool {
	if ticket < state.queue.windowTicket {
		return true
	}
	oldest, blocked := state.oldestSwitchWaiterLocked()
	return !blocked || ticket < oldest
}

func (state *activeConfigState) maySwitchLocked(ticket uint64) bool {
	oldest, blocked := state.oldestSwitchWaiterLocked()
	return blocked && oldest == ticket
}

func (state *activeConfigState) oldestSwitchWaiterLocked() (uint64, bool) {
	var oldest uint64
	found := false
	for ticket, waiter := range state.queue.waiters {
		if waiter.satisfiedBy(state) {
			continue
		}
		if !found || ticket < oldest {
			oldest = ticket
			found = true
		}
	}
	return oldest, found
}

func leaveSwitchQueue(state *activeConfigState, ticket uint64) {
	state.mu.Lock()
	state.leaveQueueLocked(ticket)
	state.mu.Unlock()
}
