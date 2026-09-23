package proxy

func (service *Service) requestsRunningOnEveryBackendFamily() int {
	active := 0
	for _, family := range service.backendFamilies {
		for _, runtime := range uniqueBackendRuntimes(family) {
			runtime.state.mu.Lock()
			active += runtime.state.users
			runtime.state.mu.Unlock()
		}
	}
	return active
}

func (scheduler *scheduler) requestsBorrowedFromPeersInFlight() int {
	return scheduler.imageQueue.BorrowedInFlight() + scheduler.textQueue.BorrowedInFlight()
}

func (scheduler *scheduler) idleForBorrowedWork() bool {
	return scheduler.deps.requestsRunningOnEveryBackendFamily()-scheduler.requestsBorrowedFromPeersInFlight() <= 0
}
