package proxy

import "time"

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

func (service *Service) backendsIdleSince() (time.Time, bool) {
	var latest time.Time
	for _, family := range service.backendFamilies {
		for _, runtime := range uniqueBackendRuntimes(family) {
			runtime.state.mu.Lock()
			users, idleSince := runtime.state.users, runtime.state.idleSince
			runtime.state.mu.Unlock()
			if users > 0 {
				return time.Time{}, false
			}
			if idleSince.After(latest) {
				latest = idleSince
			}
		}
	}
	return latest, true
}

func (scheduler *scheduler) requestsBorrowedFromPeersInFlight() int {
	return scheduler.imageQueue.BorrowedInFlight() + scheduler.textQueue.BorrowedInFlight()
}

func (scheduler *scheduler) idleForBorrowedWork() bool {
	return scheduler.deps.requestsRunningOnEveryBackendFamily()-scheduler.requestsBorrowedFromPeersInFlight() <= 0
}

func (scheduler *scheduler) idleFor(now time.Time) time.Duration {
	idleSince, idle := scheduler.deps.backendsIdleSince()
	if !idle {
		return 0
	}
	if idleSince.Before(scheduler.startedAt) {
		idleSince = scheduler.startedAt
	}
	return now.Sub(idleSince)
}
