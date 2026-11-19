package proxy

import (
	"testing"
	"time"
)

func idleTestState(filename string, generation uint64, idleSince time.Time) *activeConfigState {
	state := newActiveConfigState()
	state.filename = filename
	state.generation = generation
	state.idleSince = idleSince
	return state
}

func TestIdleUnloadStillDueTreatsAnotherQueuedRequestAsBusy(t *testing.T) {
	idleSince := time.Now()
	state := idleTestState("a.kcpps", 3, idleSince)
	state.joinQueueLocked(state.takeTicketLocked(), neverSatisfied, waitingRequest{})
	state.joinQueueLocked(state.takeTicketLocked(), neverSatisfied, waitingRequest{modelID: "b"})

	due := idleUnloadStillDue(idleRuntimeSnapshot{filename: "a.kcpps", generation: 3, idleSince: idleSince}, time.Second, idleSince.Add(time.Hour))
	if err := due(state); err != errIdleUnloadBusy {
		t.Fatalf("err=%v, want errIdleUnloadBusy for a queued request", err)
	}
}

func TestIdleUnloadStillDueAllowsTheClaimersOwnQueueEntry(t *testing.T) {
	idleSince := time.Now()
	state := idleTestState("a.kcpps", 3, idleSince)
	state.joinQueueLocked(state.takeTicketLocked(), neverSatisfied, waitingRequest{})

	due := idleUnloadStillDue(idleRuntimeSnapshot{filename: "a.kcpps", generation: 3, idleSince: idleSince}, time.Second, idleSince.Add(time.Hour))
	if err := due(state); err != nil {
		t.Fatalf("err=%v, want the claim to proceed", err)
	}
}

func TestIdleUnloadStillDueRechecksIdleTimeUnderTheLock(t *testing.T) {
	wentIdle := time.Now()
	state := idleTestState("a.kcpps", 3, wentIdle.Add(50*time.Second))

	due := idleUnloadStillDue(idleRuntimeSnapshot{filename: "a.kcpps", generation: 3, idleSince: wentIdle}, time.Minute, wentIdle.Add(70*time.Second))
	if err := due(state); err != errIdleUnloadNotDue {
		t.Fatalf("err=%v, want errIdleUnloadNotDue after a newer release", err)
	}
}

func TestIdleUnloadStillDueRejectsAChangedGeneration(t *testing.T) {
	idleSince := time.Now()
	state := idleTestState("a.kcpps", 4, idleSince)

	due := idleUnloadStillDue(idleRuntimeSnapshot{filename: "a.kcpps", generation: 3, idleSince: idleSince}, time.Second, idleSince.Add(time.Hour))
	if err := due(state); err != errIdleUnloadStale {
		t.Fatalf("err=%v, want errIdleUnloadStale", err)
	}
}
