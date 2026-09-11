package proxy

// idleForBorrowedWork is ActiveRequests' main-line-only, borrowed-exclusive
// twin: it sums activeConfigState.users live across every runtime of every
// configured backend family (never a separate pool, which
// uniqueBackendRuntimes already excludes by construction — a separate pool is
// provisioned precisely not to contend with the main line), then subtracts
// requests this node is already running on someone else's behalf, so a helper
// never looks busy from the very job it was lent.
//
// Every family, not just currentBackendMode's: a .kcpps may override
// backend_mode per model, so a borrowed job's runtime can sit in a family
// other than the one currently selected. Scoping activeRequests to only the
// current family while borrowedInFlight stays global would let unrelated
// borrowed activity in another family mask real native load in this one.
//
// A borrowed request must never delay the node that lent the slot, so this
// asks whether the whole node is idle rather than whether one lane's queue is
// — the lanes share a GPU regardless of whether they happen to run in the same
// backend process or two entirely separate ones.
func (service *Service) idleForBorrowedWork() bool {
	activeRequests := 0
	for _, family := range service.backendFamilies {
		for _, runtime := range uniqueBackendRuntimes(family) {
			runtime.state.mu.Lock()
			activeRequests += runtime.state.users
			runtime.state.mu.Unlock()
		}
	}
	borrowedInFlight := 0
	if service.imageQueue != nil {
		borrowedInFlight += service.imageQueue.BorrowedInFlight()
	}
	if service.textQueue != nil {
		borrowedInFlight += service.textQueue.BorrowedInFlight()
	}
	return activeRequests-borrowedInFlight <= 0
}
