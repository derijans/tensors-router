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

func (service *Service) requestsBorrowedFromPeersInFlight() int {
	borrowed := 0
	if service.imageQueue != nil {
		borrowed += service.imageQueue.BorrowedInFlight()
	}
	if service.textQueue != nil {
		borrowed += service.textQueue.BorrowedInFlight()
	}
	return borrowed
}

func (service *Service) idleForBorrowedWork() bool {
	return service.requestsRunningOnEveryBackendFamily()-service.requestsBorrowedFromPeersInFlight() <= 0
}
