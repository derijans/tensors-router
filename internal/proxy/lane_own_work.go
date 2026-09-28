package proxy

import "time"

func beginRuntimeSwitchLocked(state *activeConfigState, borrowed bool) {
	state.switching = true
	state.switchingForBorrowed = borrowed
}

func endRuntimeSwitchLocked(state *activeConfigState) {
	if !state.switchingForBorrowed {
		state.ownIdleSince = time.Now()
	}
	state.switching = false
	state.switchingForBorrowed = false
}

func (service *Service) addRuntimeLeaseLocked(state *activeConfigState, modelID string, borrowed bool) func() {
	state.users++
	if borrowed {
		state.borrowedUsers++
	}
	leaseTag := service.nextRuntimeLease.Add(1)
	state.leases[leaseTag] = modelID
	return releaseActiveConfigLeaseOnce(state, leaseTag, borrowed)
}

func (state *activeConfigState) busyWithOwnWorkLocked() bool {
	return state.users-state.borrowedUsers > 0 || state.switching && !state.switchingForBorrowed
}
