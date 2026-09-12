package proxy

import (
	"context"
	"sync"
	"time"
)

const defaultOffloadRestoreDelay = 1500 * time.Millisecond

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

func (service *Service) borrowRestoreStateFor(runtime *backendRuntime) *borrowRestoreState {
	value, _ := service.borrowRestore.LoadOrStore(runtime, &borrowRestoreState{})
	return value.(*borrowRestoreState)
}

func (service *Service) noteBorrowRestoreActivity(ctx context.Context, runtime *backendRuntime, mode string, incomingFilename string, readiness backendReadiness) {
	if !contextIsBorrowed(ctx) {
		service.clearBorrowRestore(runtime)
		return
	}
	if contextBorrowRestoreRequested(ctx) {
		if modelID, filename := runtime.state.loadedModel(); filename != "" && filename != incomingFilename {
			service.recordFirstDisplacement(runtime, mode, modelID, filename, readiness)
		}
	}
	service.touchBorrowRestore(runtime)
}

func (service *Service) recordFirstDisplacement(runtime *backendRuntime, mode string, modelID string, filename string, readiness backendReadiness) {
	state := service.borrowRestoreStateFor(runtime)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.target != nil {
		return
	}
	state.target = &displacedConfig{mode: mode, modelID: modelID, filename: filename, readiness: readiness}
}

func (service *Service) touchBorrowRestore(runtime *backendRuntime) {
	state := service.borrowRestoreStateFor(runtime)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.target == nil {
		return
	}
	delay := service.offloadRestoreDelay
	if delay <= 0 {
		delay = defaultOffloadRestoreDelay
	}
	if state.timer != nil {
		state.timer.Stop()
	}
	state.timer = time.AfterFunc(delay, func() { service.fireBorrowRestore(runtime, state) })
}

func (service *Service) clearBorrowRestore(runtime *backendRuntime) {
	state := service.borrowRestoreStateFor(runtime)
	state.mu.Lock()
	defer state.mu.Unlock()
	stopBorrowRestoreLocked(state)
}

func stopBorrowRestoreLocked(state *borrowRestoreState) {
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	state.target = nil
}

// nodeActivity().idle() alone can be true with a borrowed request still running, since
// idleForBorrowedWork nets that activity out; the two BorrowedInFlight checks catch it.
func (service *Service) borrowRestoreQuiet() bool {
	if service.imageQueue != nil && service.imageQueue.BorrowedInFlight() > 0 {
		return false
	}
	if service.textQueue != nil && service.textQueue.BorrowedInFlight() > 0 {
		return false
	}
	return service.nodeActivity().idle()
}

func (service *Service) fireBorrowRestore(runtime *backendRuntime, state *borrowRestoreState) {
	state.mu.Lock()
	target := state.target
	state.mu.Unlock()
	if target == nil {
		return
	}
	if !service.borrowRestoreQuiet() {
		return
	}

	_, release, _, err := service.acquireModelConfigForBackendMode(target.mode, context.Background(), target.modelID, target.filename, target.readiness, false)

	state.mu.Lock()
	if state.target == target {
		stopBorrowRestoreLocked(state)
	}
	state.mu.Unlock()

	if err != nil {
		service.logger.Printf("borrow restore failed mode=%s config=%q error=%v", target.mode, target.filename, err)
		return
	}
	release()
}

func contextIsBorrowed(ctx context.Context) bool {
	borrowed, _ := ctx.Value(offloadContextKey{}).(bool)
	return borrowed
}

func contextBorrowRestoreRequested(ctx context.Context) bool {
	requested, _ := ctx.Value(offloadRestoreContextKey{}).(bool)
	return requested
}
