package proxy

import (
	"errors"
	"time"
)

var (
	errIdleUnloadBusy   = errors.New("runtime is serving or awaited")
	errIdleUnloadStale  = errors.New("runtime changed config since it went idle")
	errIdleUnloadNotDue = errors.New("runtime has not been idle long enough")
)

const claimersOwnQueueEntry = 1

type idleRuntimeSnapshot struct {
	filename   string
	generation uint64
	idleSince  time.Time
}

func idleRuntimeSnapshotOf(state *activeConfigState) (idleRuntimeSnapshot, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	snapshot := idleRuntimeSnapshot{filename: state.filename, generation: state.generation, idleSince: state.idleSince}
	idle := !state.switching && state.users == 0 && state.filename != "" && state.queuedWaitersLocked() == 0
	return snapshot, idle
}

func idleUnloadStillDue(expected idleRuntimeSnapshot, timeout time.Duration, now time.Time) func(*activeConfigState) error {
	return func(state *activeConfigState) error {
		if state.switching || state.users > 0 || state.queuedWaitersLocked() > claimersOwnQueueEntry {
			return errIdleUnloadBusy
		}
		if state.filename == "" || state.filename != expected.filename || state.generation != expected.generation {
			return errIdleUnloadStale
		}
		if now.Sub(state.idleSince) < timeout {
			return errIdleUnloadNotDue
		}
		return nil
	}
}

func idleUnloadSkipped(err error) bool {
	return errors.Is(err, errIdleUnloadBusy) || errors.Is(err, errIdleUnloadStale) || errors.Is(err, errIdleUnloadNotDue)
}
