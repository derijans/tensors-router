package proxy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

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
	physicalFilename    string
	physicalFingerprint string
	physicalShareable   bool
	physicalAttemptID   string
	// pendingFilename and pendingProfile describe the config currently being loaded while
	// switching is true. Without them a concurrent caller wanting that same config sees
	// only "switching" and concludes the runtime holds something else, then unloads it —
	// killing the load in flight.
	pendingFilename   string
	pendingProfile    catalog.ChatTemplateProfile
	users             int
	switching         bool
	switchWaiters     int
	vramBaselineMB    int64
	vramTotalMB       int64
	vramBaselineValid bool
	modelID           string
	generation        uint64
	leases            map[uint64]string
}

func newActiveConfigState() *activeConfigState {
	return &activeConfigState{changed: make(chan struct{}), leases: map[uint64]string{}}
}

func (service *Service) acquireModelConfigForBackendMode(mode string, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool) (*backendRuntime, func(), bool, error) {
	return service.acquireModelConfigForBackendModeWithOptions(mode, ctx, modelID, configFilename, readiness, modelConfigAcquireOptions{forceReload: force})
}

func (service *Service) acquireExactModelConfigForBackendMode(mode string, ctx context.Context, modelID string, configFilename string, readiness backendReadiness) (*backendRuntime, func(), bool, error) {
	return service.acquireModelConfigForBackendModeWithOptions(mode, ctx, modelID, configFilename, readiness, modelConfigAcquireOptions{exactPhysicalConfig: true})
}

type modelConfigAcquireOptions struct {
	forceReload         bool
	exactPhysicalConfig bool
}

func (service *Service) acquireModelConfigForBackendModeWithOptions(mode string, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, options modelConfigAcquireOptions) (*backendRuntime, func(), bool, error) {
	embeddingRequest := readiness == readinessEmbeddings
	resolvedReadiness, err := service.readinessForConfig(configFilename, readiness)
	if err != nil {
		return nil, nil, false, err
	}
	readiness = resolvedReadiness
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
	if embeddingRequest {
		return service.acquireEmbeddingConfig(runtime, ctx, modelID, configFilename, readiness, options, finishDiagnostic)
	}
	if err := service.ensureBackendFamily(ctx, mode); err != nil {
		return nil, nil, false, service.backendLoadDiagnosticError(err, runtime, finishDiagnostic)
	}
	if err := service.enforceUnloadPolicy(ctx, mode, configFilename, readiness); err != nil {
		return nil, nil, false, service.backendLoadDiagnosticError(err, runtime, finishDiagnostic)
	}
	release, loadedFresh, err := service.acquireModelConfigWithOptions(runtime, ctx, modelID, configFilename, readiness, options)
	if err != nil {
		return runtime, nil, false, service.backendLoadDiagnosticError(err, runtime, finishDiagnostic)
	}
	finishDiagnostic(true)
	return runtime, release, loadedFresh, err
}

func (service *Service) acquireEmbeddingConfig(runtime *backendRuntime, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, options modelConfigAcquireOptions, finishDiagnostic func(bool) backenddiagnostic.Diagnostic) (*backendRuntime, func(), bool, error) {
	service.embeddingSelection.mu.Lock()
	defer service.embeddingSelection.mu.Unlock()
	previous := service.embeddingSelection.runtime
	if previous != nil && previous != runtime {
		if err := service.unloadRuntime(ctx, previous); err != nil {
			return nil, nil, false, service.backendLoadDiagnosticError(err, runtime, finishDiagnostic)
		}
		service.embeddingSelection.runtime = nil
	}
	release, loadedFresh, err := service.acquireModelConfigWithOptions(runtime, ctx, modelID, configFilename, readiness, options)
	if err != nil {
		return runtime, nil, false, service.backendLoadDiagnosticError(err, runtime, finishDiagnostic)
	}
	service.embeddingSelection.runtime = runtime
	finishDiagnostic(true)
	return runtime, release, loadedFresh, nil
}

func (service *Service) readinessForConfig(filename string, readiness backendReadiness) (backendReadiness, error) {
	if readiness != readinessEmbeddings {
		return readiness, nil
	}
	if filename == "" {
		return readiness, nil
	}
	if filename != filepath.Base(filename) {
		return readiness, fmt.Errorf("config filename %q is invalid", filename)
	}
	if service.catalog != nil {
		models, err := service.catalog.List()
		if err != nil {
			return readiness, err
		}
		for _, model := range models {
			if model.Filename != filename {
				continue
			}
			if model.Capabilities.Embeddings != nil && model.Capabilities.Embeddings.Separate {
				return readinessEmbeddings, nil
			}
			return readinessText, nil
		}
	}
	if service.configDir == "" {
		return readinessText, nil
	}
	metadata, err := catalog.LoadRuntimeConfig(filepath.Join(service.configDir, filename))
	if err != nil {
		if os.IsNotExist(err) {
			return readinessText, nil
		}
		return readiness, err
	}
	if !metadata.RunEmbedSeparate {
		return readinessText, nil
	}
	return readinessEmbeddings, nil
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
	return service.acquireModelConfigWithOptions(runtime, ctx, modelID, configFilename, readiness, modelConfigAcquireOptions{forceReload: force})
}

func (service *Service) acquireModelConfigWithOptions(runtime *backendRuntime, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, options modelConfigAcquireOptions) (func(), bool, error) {
	waitingSwitch := false
	state := runtime.state
	profile := service.chatTemplateProfileForConfig(configFilename)
	for {
		state.mu.Lock()
		if !options.forceReload && activeConfigMatchesAcquireOptions(state, configFilename, profile, options) && !state.switching && (state.switchWaiters == 0 || waitingSwitch) {
			if waitingSwitch {
				state.switchWaiters--
				notifyActiveConfigLocked(state)
			}
			logicalConfigChanged := state.filename != configFilename
			logicalModelChanged := state.modelID != modelID
			state.filename = configFilename
			state.modelID = modelID
			if logicalConfigChanged || logicalModelChanged {
				state.generation++
			}
			state.users++
			leaseTag := service.nextRuntimeLease.Add(1)
			state.leases[leaseTag] = modelID
			physicalAttemptID := state.physicalAttemptID
			release := releaseActiveConfigLeaseOnce(state, leaseTag)
			state.mu.Unlock()
			service.recordLoadReuse(physicalAttemptID)
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
		state.pendingFilename = configFilename
		state.pendingProfile = profile
		state.mu.Unlock()

		capture, err := service.beginPhysicalLoadCapture(ctx, runtime, configFilename, readiness)
		if err != nil {
			state.mu.Lock()
			state.switching = false
			state.pendingFilename = ""
			state.pendingProfile = catalog.ChatTemplateProfile{}
			state.filename = ""
			state.modelID = ""
			state.physicalAttemptID = ""
			clearPhysicalLoadProfileLocked(state)
			clearVRAMLoadStateLocked(state)
			notifyActiveConfigLocked(state)
			state.mu.Unlock()
			service.invalidateWebUIRoutes()
			return nil, false, err
		}
		vramLoad := service.beginVRAMLoad(ctx)
		err = service.reloadModelConfig(runtime, ctx, modelID, configFilename)
		if err == nil {
			err = service.waitForBackendEndpoint(runtime, ctx, readiness, modelID, configFilename)
		}
		service.finishVRAMLoad(ctx, vramLoad)
		service.finishPhysicalLoadCapture(capture, err)

		state.mu.Lock()
		state.switching = false
		state.pendingFilename = ""
		state.pendingProfile = catalog.ChatTemplateProfile{}
		if err != nil {
			state.filename = ""
			state.modelID = ""
			state.physicalAttemptID = ""
			clearPhysicalLoadProfileLocked(state)
			clearVRAMLoadStateLocked(state)
			notifyActiveConfigLocked(state)
			state.mu.Unlock()
			service.invalidateWebUIRoutes()
			return nil, false, err
		}
		state.filename = configFilename
		state.modelID = modelID
		state.generation++
		if capture != nil {
			state.physicalAttemptID = capture.attempt.ID
		} else {
			state.physicalAttemptID = ""
		}
		applyPhysicalLoadProfileLocked(state, configFilename, profile)
		applyVRAMLoadStateLocked(state, vramLoad)
		state.users++
		leaseTag := service.nextRuntimeLease.Add(1)
		state.leases[leaseTag] = modelID
		release := releaseActiveConfigLeaseOnce(state, leaseTag)
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

		state.modelID = ""
		state.generation++
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

		state.modelID = ""
		state.generation++
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
	return releaseActiveConfigLeaseOnce(state, 0)
}

func releaseActiveConfigLeaseOnce(state *activeConfigState, leaseTag uint64) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			state.mu.Lock()
			if leaseTag != 0 {
				delete(state.leases, leaseTag)
			}
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

func activeConfigMatchesAcquireOptions(state *activeConfigState, filename string, profile catalog.ChatTemplateProfile, options modelConfigAcquireOptions) bool {
	if options.exactPhysicalConfig {
		return state.physicalFilename == filename
	}
	return activeConfigMatchesProfile(state, filename, profile)
}

func activeRuntimeSupportsConfig(runtime *backendRuntime, filename string, profile catalog.ChatTemplateProfile) bool {
	if runtime == nil {
		return false
	}
	runtime.state.mu.Lock()
	defer runtime.state.mu.Unlock()
	if runtime.state.switching {
		// A load is in flight. If it is loading what this caller wants, the runtime does
		// support the config: report that rather than letting the caller unload it and
		// abort the load partway through.
		return pendingConfigMatchesProfileLocked(runtime.state, filename, profile)
	}
	if runtime.state.filename == "" {
		return false
	}
	return activeConfigMatchesProfile(runtime.state, filename, profile)
}

func pendingConfigMatchesProfileLocked(state *activeConfigState, filename string, profile catalog.ChatTemplateProfile) bool {
	if state.pendingFilename == "" {
		return false
	}
	if state.pendingFilename == filename {
		return true
	}
	// Profile variants of one physical model share a runtime, so a pending load of a
	// sibling variant also satisfies this caller.
	fingerprint := profile.PhysicalLoadFingerprint()
	return profile.HasConfiguredKwargs() && fingerprint != "" &&
		state.pendingProfile.HasConfiguredKwargs() && state.pendingProfile.PhysicalLoadFingerprint() == fingerprint
}

func applyPhysicalLoadProfileLocked(state *activeConfigState, filename string, profile catalog.ChatTemplateProfile) {
	state.physicalFilename = filename
	state.physicalFingerprint = profile.PhysicalLoadFingerprint()
	state.physicalShareable = profile.Valid() && profile.HasConfiguredKwargs() && state.physicalFingerprint != ""
}

func clearPhysicalLoadProfileLocked(state *activeConfigState) {
	state.physicalFilename = ""
	state.physicalFingerprint = ""
	state.physicalAttemptID = ""
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
