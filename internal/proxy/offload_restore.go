package proxy

import (
	"context"
	"sync"
	"time"
)

type displacedConfig struct {
	mode      string
	modelID   string
	filename  string
	readiness backendReadiness
}

type borrowRestoreState struct {
	mu     sync.Mutex
	target *displacedConfig
	timer  *time.Timer
}

func (scheduler *scheduler) borrowRestoreStateFor(runtime *backendRuntime) *borrowRestoreState {
	value, _ := scheduler.borrowRestore.LoadOrStore(runtime, &borrowRestoreState{})
	return value.(*borrowRestoreState)
}

func (scheduler *scheduler) noteBorrowRestoreActivity(ctx context.Context, runtime *backendRuntime, mode string, incomingFilename string) {
	if !contextIsBorrowed(ctx) {
		scheduler.clearBorrowRestore(runtime)
		return
	}
	if contextBorrowRestoreRequested(ctx) {
		if modelID, filename, loadedReadiness := runtime.state.loadedModelWithReadiness(); filename != "" && filename != incomingFilename {
			scheduler.recordFirstDisplacement(runtime, mode, modelID, filename, loadedReadiness)
		}
	}
	scheduler.touchBorrowRestore(runtime)
}

func (scheduler *scheduler) recordFirstDisplacement(runtime *backendRuntime, mode string, modelID string, filename string, readiness backendReadiness) {
	state := scheduler.borrowRestoreStateFor(runtime)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.target != nil {
		return
	}
	state.target = &displacedConfig{mode: mode, modelID: modelID, filename: filename, readiness: readiness}
}

func (scheduler *scheduler) touchBorrowRestore(runtime *backendRuntime) {
	state := scheduler.borrowRestoreStateFor(runtime)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.target == nil {
		return
	}
	delay := scheduler.currentSettings().RestoreDelay
	if state.timer != nil {
		state.timer.Stop()
	}
	state.timer = time.AfterFunc(delay, func() { scheduler.fireBorrowRestore(runtime, state) })
}

func (scheduler *scheduler) clearBorrowRestore(runtime *backendRuntime) {
	state := scheduler.borrowRestoreStateFor(runtime)
	state.mu.Lock()
	defer state.mu.Unlock()
	stopBorrowRestoreLocked(state)
}

func (scheduler *scheduler) stopBorrowRestores() {
	scheduler.borrowRestore.Range(func(_, value any) bool {
		state := value.(*borrowRestoreState)
		state.mu.Lock()
		defer state.mu.Unlock()
		stopBorrowRestoreLocked(state)
		return true
	})
}

func stopBorrowRestoreLocked(state *borrowRestoreState) {
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	state.target = nil
}

// nodeActivity().idle() alone can be true with a borrowed request still running, since
// idleForBorrowedWork nets that activity out; the borrowed in-flight count catches it.
func (scheduler *scheduler) borrowRestoreQuiet() bool {
	return scheduler.requestsBorrowedFromPeersInFlight() == 0 && scheduler.nodeActivity().idle()
}

func (scheduler *scheduler) fireBorrowRestore(runtime *backendRuntime, state *borrowRestoreState) {
	state.mu.Lock()
	target := state.target
	state.mu.Unlock()
	if target == nil {
		return
	}
	if !scheduler.borrowRestoreQuiet() {
		scheduler.retryBorrowRestoreLater(runtime, state, target)
		return
	}

	scheduler.logger.Printf("borrow restore loading mode=%s config=%q section=%s", target.mode, target.filename, readinessAnalyticsSection(target.readiness))
	started := time.Now()
	_, release, _, err := scheduler.deps.acquireModelConfigForBackendMode(target.mode, context.Background(), target.modelID, target.filename, target.readiness, false)

	state.mu.Lock()
	if state.target == target {
		stopBorrowRestoreLocked(state)
	}
	state.mu.Unlock()

	if err != nil {
		scheduler.logger.Printf("borrow restore failed mode=%s config=%q elapsed=%s error=%v", target.mode, target.filename, time.Since(started), err)
		return
	}
	scheduler.logger.Printf("borrow restore finished mode=%s config=%q elapsed=%s", target.mode, target.filename, time.Since(started))
	release()
}

func (scheduler *scheduler) retryBorrowRestoreLater(runtime *backendRuntime, state *borrowRestoreState, target *displacedConfig) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.target != target {
		return
	}
	state.timer = time.AfterFunc(scheduler.currentSettings().RestoreDelay, func() { scheduler.fireBorrowRestore(runtime, state) })
}

func contextIsBorrowed(ctx context.Context) bool {
	borrowed, _ := ctx.Value(offloadContextKey{}).(bool)
	return borrowed
}

func contextBorrowRestoreRequested(ctx context.Context) bool {
	requested, _ := ctx.Value(offloadRestoreContextKey{}).(bool)
	return requested
}
