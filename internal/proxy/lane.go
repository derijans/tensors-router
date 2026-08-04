package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tensors-router/internal/auth"
	"tensors-router/internal/backenddiagnostic"
	"tensors-router/internal/catalog"
)

type backendRuntime struct {
	backend Backend
	state   *activeConfigState
	mode    string
	name    string
}

type activeConfigState struct {
	mu                  sync.Mutex
	changed             chan struct{}
	filename            string
	physicalFingerprint string
	physicalShareable   bool
	users               int
	switching           bool
	switchWaiters       int
	vramBaselineMB      int64
	vramTotalMB         int64
	vramBaselineValid   bool
}

func newActiveConfigState() *activeConfigState {
	return &activeConfigState{changed: make(chan struct{})}
}

func (service *Service) acquireModelConfigForBackendMode(mode string, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool) (*backendRuntime, func(), bool, error) {
	if err := service.requireMCPAdmin(ctx, configFilename); err != nil {
		return nil, nil, false, err
	}
	if err := service.ensureModelConfigHash(configFilename); err != nil {
		return nil, nil, false, err
	}
	if err := service.ensureModelAssets(ctx, configFilename); err != nil {
		return nil, nil, false, err
	}
	runtime, err := service.runtimeForBackendMode(mode, readiness)
	if err != nil {
		return nil, nil, false, err
	}
	finishDiagnostic := beginBackendDiagnostic(runtime.backend)
	if err := service.ensureBackendFamily(ctx, mode); err != nil {
		return nil, nil, false, service.backendLoadDiagnosticError(err, runtime, finishDiagnostic)
	}
	if err := service.enforceUnloadPolicy(ctx, mode, configFilename); err != nil {
		return nil, nil, false, service.backendLoadDiagnosticError(err, runtime, finishDiagnostic)
	}
	release, loadedFresh, err := service.acquireModelConfig(runtime, ctx, modelID, configFilename, readiness, force)
	if err != nil {
		return runtime, nil, false, service.backendLoadDiagnosticError(err, runtime, finishDiagnostic)
	}
	finishDiagnostic(true)
	return runtime, release, loadedFresh, err
}

func (service *Service) requireMCPAdmin(ctx context.Context, filename string) error {
	if service.mcpReconciler == nil || !service.mcpReconciler.Enabled() || service.catalog == nil {
		return nil
	}
	models, err := service.catalog.List()
	if err != nil {
		return err
	}
	for _, model := range models {
		if model.Filename == filename && model.MCPEnabled && !auth.PrincipalFromContext(ctx).Admin {
			return fmt.Errorf("MCP-enabled models require an authenticated admin principal")
		}
	}
	return nil
}

func (service *Service) requireActiveMCPAdmin(ctx context.Context) error {
	for _, runtime := range []*backendRuntime{service.textRuntime, service.imageRuntime} {
		if runtime == nil {
			continue
		}
		if err := service.requireMCPAdmin(ctx, currentRuntimeConfigFilename(runtime)); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) ensureModelConfigHash(filename string) error {
	hasher, ok := service.catalog.(modelHashEnsurer)
	if !ok {
		return nil
	}
	started := time.Now()
	_, scanned, err := hasher.EnsureModelHashForFilename(filename)
	if err != nil {
		service.logger.Printf("model config scan failed config=%q elapsed=%s error=%v", filename, time.Since(started), err)
		return err
	}
	if !scanned {
		return nil
	}
	if service.registry != nil {
		models, registryErr := service.localClusterModels()
		if registryErr != nil {
			service.logger.Printf("model config registry refresh failed config=%q elapsed=%s error=%v", filename, time.Since(started), registryErr)
			return registryErr
		}
		if registryErr := service.registry.UpdateLocal(models); registryErr != nil {
			service.logger.Printf("model config registry refresh failed config=%q elapsed=%s error=%v", filename, time.Since(started), registryErr)
			return registryErr
		}
	}
	service.logger.Printf("model config scan completed config=%q elapsed=%s", filename, time.Since(started))
	return nil
}

func (service *Service) acquireModelConfig(runtime *backendRuntime, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool) (func(), bool, error) {
	waitingSwitch := false
	state := runtime.state
	profile := service.chatTemplateProfileForConfig(configFilename)
	for {
		state.mu.Lock()
		if !force && activeConfigMatchesProfile(state, configFilename, profile) && !state.switching && (state.switchWaiters == 0 || waitingSwitch) {
			if waitingSwitch {
				state.switchWaiters--
				notifyActiveConfigLocked(state)
			}
			logicalConfigChanged := state.filename != configFilename
			state.filename = configFilename
			state.users++
			release := releaseActiveConfigOnce(state)
			state.mu.Unlock()
			if logicalConfigChanged {
				service.invalidateWebUIRoutes()
			}
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

		vramLoad := service.beginVRAMLoad(ctx)
		err := service.reloadModelConfig(runtime, ctx, modelID, configFilename)
		if err == nil {
			err = service.waitForBackendEndpoint(runtime, ctx, readiness, modelID, configFilename)
		}
		service.finishVRAMLoad(ctx, vramLoad)

		state.mu.Lock()
		state.switching = false
		if err != nil {
			state.filename = ""
			clearPhysicalLoadProfileLocked(state)
			clearVRAMLoadStateLocked(state)
			notifyActiveConfigLocked(state)
			state.mu.Unlock()
			service.invalidateWebUIRoutes()
			return nil, false, err
		}
		state.filename = configFilename
		applyPhysicalLoadProfileLocked(state, profile)
		applyVRAMLoadStateLocked(state, vramLoad)
		state.users++
		release := releaseActiveConfigOnce(state)
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		service.recordVRAMLoad(modelID, configFilename, readiness, runtime.mode, vramLoad)
		service.invalidateWebUIRoutes()
		return release, true, nil
	}
}

type backendDiagnosticRecorder interface {
	BeginLoadDiagnostic() func(bool) backenddiagnostic.Diagnostic
}

func beginBackendDiagnostic(backend Backend) func(bool) backenddiagnostic.Diagnostic {
	if recorder, ok := backend.(backendDiagnosticRecorder); ok {
		return recorder.BeginLoadDiagnostic()
	}
	return func(bool) backenddiagnostic.Diagnostic { return backenddiagnostic.Diagnostic{} }
}

func (service *Service) backendLoadDiagnosticError(err error, runtime *backendRuntime, finish func(bool) backenddiagnostic.Diagnostic) error {
	diagnostic := finish(false)
	diagnostic.NodeID = service.nodeID
	diagnostic.Backend = runtime.name
	return backenddiagnostic.WithDiagnostic(err, diagnostic)
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
		clearPhysicalLoadProfileLocked(state)
		clearVRAMLoadStateLocked(state)
		notifyActiveConfigLocked(state)
		state.mu.Unlock()

		err := runtime.backend.Unload(ctx)

		state.mu.Lock()
		state.switching = false
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		service.invalidateWebUIRoutes()
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
		clearPhysicalLoadProfileLocked(state)
		clearVRAMLoadStateLocked(state)
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

func activeConfigMatchesProfile(state *activeConfigState, filename string, profile catalog.ChatTemplateProfile) bool {
	if state.filename == filename {
		return true
	}
	return state.physicalShareable && profile.HasConfiguredKwargs() && state.physicalFingerprint != "" && state.physicalFingerprint == profile.PhysicalLoadFingerprint()
}

func activeRuntimeSupportsConfig(runtime *backendRuntime, filename string, profile catalog.ChatTemplateProfile) bool {
	if runtime == nil {
		return false
	}
	runtime.state.mu.Lock()
	defer runtime.state.mu.Unlock()
	if runtime.state.switching || runtime.state.filename == "" {
		return false
	}
	return activeConfigMatchesProfile(runtime.state, filename, profile)
}

func applyPhysicalLoadProfileLocked(state *activeConfigState, profile catalog.ChatTemplateProfile) {
	state.physicalFingerprint = profile.PhysicalLoadFingerprint()
	state.physicalShareable = profile.Valid() && profile.HasConfiguredKwargs() && state.physicalFingerprint != ""
}

func clearPhysicalLoadProfileLocked(state *activeConfigState) {
	state.physicalFingerprint = ""
	state.physicalShareable = false
}

func (service *Service) chatTemplateProfileForConfig(filename string) catalog.ChatTemplateProfile {
	if service.catalog == nil || filename == "" {
		return catalog.ChatTemplateProfile{}
	}
	models, err := service.catalog.List()
	if err != nil {
		return catalog.ChatTemplateProfile{}
	}
	for _, model := range models {
		if model.Filename == filename {
			return model.ChatTemplate
		}
	}
	return catalog.ChatTemplateProfile{}
}
