package proxy

import (
	"context"
	"sync"
)

type backendRuntime struct {
	backend Backend
	state   *activeConfigState
	name    string
}

type activeConfigState struct {
	mu            sync.Mutex
	changed       chan struct{}
	filename      string
	users         int
	switching     bool
	switchWaiters int
}

func newActiveConfigState() *activeConfigState {
	return &activeConfigState{changed: make(chan struct{})}
}

func (service *Service) acquireModelConfigForBackendMode(mode string, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool, unloadPolicy string) (*backendRuntime, func(), bool, error) {
	if err := service.ensureBackendFamily(ctx, mode); err != nil {
		return nil, nil, false, err
	}
	if err := service.enforceUnloadPolicy(ctx, mode, configFilename, unloadPolicy); err != nil {
		return nil, nil, false, err
	}
	runtime, err := service.runtimeForBackendMode(mode, readiness)
	if err != nil {
		return nil, nil, false, err
	}
	release, loadedFresh, err := service.acquireModelConfig(runtime, ctx, modelID, configFilename, readiness, force)
	return runtime, release, loadedFresh, err
}

func (service *Service) acquireModelConfig(runtime *backendRuntime, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool) (func(), bool, error) {
	waitingSwitch := false
	state := runtime.state
	for {
		state.mu.Lock()
		if !force && state.filename == configFilename && !state.switching && (state.switchWaiters == 0 || waitingSwitch) {
			if waitingSwitch {
				state.switchWaiters--
				notifyActiveConfigLocked(state)
			}
			state.users++
			release := releaseActiveConfigOnce(state)
			state.mu.Unlock()
			return release, false, nil
		}

		if !waitingSwitch && state.switchWaiters > 0 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				return nil, false, err
			}
			continue
		}
		if !waitingSwitch {
			state.switchWaiters++
			waitingSwitch = true
		}
		if state.switching || state.users > 0 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				cancelConfigSwitchWaiter(state)
				return nil, false, err
			}
			continue
		}

		state.switchWaiters--
		state.switching = true
		state.mu.Unlock()

		err := service.reloadModelConfig(runtime, ctx, modelID, configFilename)
		if err == nil {
			err = service.waitForBackendEndpoint(runtime, ctx, readiness, modelID, configFilename)
		}

		state.mu.Lock()
		state.switching = false
		if err != nil {
			state.filename = ""
			notifyActiveConfigLocked(state)
			state.mu.Unlock()
			return nil, false, err
		}
		state.filename = configFilename
		state.users++
		release := releaseActiveConfigOnce(state)
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		return release, true, nil
	}
}

func (service *Service) unloadRuntime(ctx context.Context, runtime *backendRuntime) error {
	waitingSwitch := false
	state := runtime.state
	for {
		state.mu.Lock()
		if !waitingSwitch && state.switchWaiters > 0 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				return err
			}
			continue
		}
		if !waitingSwitch {
			state.switchWaiters++
			waitingSwitch = true
		}
		if state.switching || state.users > 0 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				cancelConfigSwitchWaiter(state)
				return err
			}
			continue
		}

		state.switchWaiters--
		state.switching = true
		state.filename = ""
		notifyActiveConfigLocked(state)
		state.mu.Unlock()

		err := runtime.backend.Unload(ctx)

		state.mu.Lock()
		state.switching = false
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		return err
	}
}

func lockRuntimeForBackendStop(ctx context.Context, runtime *backendRuntime) (func(), error) {
	if runtime == nil {
		return func() {}, nil
	}
	waitingSwitch := false
	state := runtime.state
	for {
		state.mu.Lock()
		if !waitingSwitch && state.switchWaiters > 0 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				return nil, err
			}
			continue
		}
		if !waitingSwitch {
			state.switchWaiters++
			waitingSwitch = true
		}
		if state.switching || state.users > 0 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				cancelConfigSwitchWaiter(state)
				return nil, err
			}
			continue
		}

		state.switchWaiters--
		state.switching = true
		state.filename = ""
		notifyActiveConfigLocked(state)
		state.mu.Unlock()

		return func() {
			state.mu.Lock()
			state.switching = false
			notifyActiveConfigLocked(state)
			state.mu.Unlock()
		}, nil
	}
}

func cancelConfigSwitchWaiter(state *activeConfigState) {
	state.mu.Lock()
	if state.switchWaiters > 0 {
		state.switchWaiters--
		notifyActiveConfigLocked(state)
	}
	state.mu.Unlock()
}

func releaseActiveConfigOnce(state *activeConfigState) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			state.mu.Lock()
			if state.users > 0 {
				state.users--
				if state.users == 0 {
					notifyActiveConfigLocked(state)
				}
			}
			state.mu.Unlock()
		})
	}
}

func notifyActiveConfigLocked(state *activeConfigState) {
	close(state.changed)
	state.changed = make(chan struct{})
}

func waitForActiveConfigChange(ctx context.Context, changed <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	}
}

func currentRuntimeConfigFilename(runtime *backendRuntime) string {
	runtime.state.mu.Lock()
	defer runtime.state.mu.Unlock()
	return runtime.state.filename
}
