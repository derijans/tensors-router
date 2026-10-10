package proxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tensors-router/internal/backenddiagnostic"
	"tensors-router/internal/catalog"
	"tensors-router/internal/loaderrors"
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
	physicalReadiness   backendReadiness
	// pendingFilename and pendingProfile describe the config currently being loaded while
	// switching is true. Without them a concurrent caller wanting that same config sees
	// only "switching" and concludes the runtime holds something else, then unloads it —
	// killing the load in flight.
	pendingFilename      string
	pendingProfile       catalog.ChatTemplateProfile
	users                int
	borrowedUsers        int
	ownIdleSince         time.Time
	switching            bool
	switchingForBorrowed bool
	queue                switchQueue
	vramBaselineMB       int64
	vramTotalMB          int64
	vramBaselineValid    bool
	memoryLoadedMB       int64
	modelID              string
	generation           uint64
	leases               map[uint64]string
}

func newActiveConfigState() *activeConfigState {
	return &activeConfigState{changed: make(chan struct{}), leases: map[uint64]string{}}
}

func (state *activeConfigState) loadedModel() (string, string) {
	modelID, filename, _ := state.loadedModelWithReadiness()
	return modelID, filename
}

func (state *activeConfigState) loadedModelWithReadiness() (string, string, backendReadiness) {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.switching {
		return "", "", readinessText
	}
	return state.modelID, state.filename, state.physicalReadiness
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
	resolvedReadiness, err := service.readinessForConfig(configFilename, readiness)
	if err != nil {
		service.recordLoadErrorFromErr(loaderrors.PhaseConfigParse, "proxy.readinessForConfig", configFilename, err)
		return nil, nil, false, err
	}
	readiness = resolvedReadiness
	if err := service.ensureModelConfigHash(configFilename); err != nil {
		service.recordLoadErrorFromErr(loaderrors.PhaseConfigParse, "proxy.ensureModelConfigHash", configFilename, err)
		return nil, nil, false, err
	}
	if err := service.assets.ensure(ctx, configFilename); err != nil {
		service.recordLoadErrorFromErr(loaderrors.PhaseAssetResolve, "proxy.ensureModelAssets", configFilename, err)
		return nil, nil, false, err
	}

	if handled, poolRuntime, release, loadedFresh, err := service.tryAcquireSeparateConfig(mode, ctx, modelID, configFilename, readiness, options); handled {
		if err != nil {
			return poolRuntime, nil, false, err
		}
		return poolRuntime, release, loadedFresh, nil
	}

	runtime, err := service.runtimeForBackendMode(mode, readiness)
	if err != nil {
		return nil, nil, false, err
	}
	finishDiagnostic := beginBackendDiagnostic(runtime.backend)
	releaseFamily, err := service.leaseBackendFamily(ctx, mode)
	if err != nil {
		return nil, nil, false, service.backendLoadDiagnosticError(err, runtime, modelID, configFilename, finishDiagnostic)
	}
	defer releaseFamily()
	service.scheduler.noteBorrowRestoreActivity(ctx, runtime, mode, configFilename)
	if err := service.enforceUnloadPolicy(ctx, mode, configFilename, readiness); err != nil {
		return nil, nil, false, service.backendLoadDiagnosticError(err, runtime, modelID, configFilename, finishDiagnostic)
	}
	release, loadedFresh, err := service.acquireModelConfigWithOptions(runtime, ctx, modelID, configFilename, readiness, options)
	if err != nil {
		return runtime, nil, false, service.backendLoadDiagnosticError(err, runtime, modelID, configFilename, finishDiagnostic)
	}
	service.applySeparateRuntimeTriggers(ctx, mode, configFilename, modelID)
	finishDiagnostic(true)
	return runtime, release, loadedFresh, err
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

// loadModelConfig applies a config and waits for the backend to serve it, re-issuing the
// reload if the backend comes up without the model.
//
// KoboldCpp restarts itself to apply an admin reload, and a reload issued while it is
// still coming up is accepted — it answers success — but lost: the process returns in
// no-model mode and sits at "inactive" indefinitely. Nothing about that is distinguishable
// from a slow load at the moment it happens, and the backend never corrects itself. Only
// another reload does. Observed directly: with a single attempt, five KoboldCpp starts
// produced zero completed loads; the load only ever landed once a later request happened
// to issue a fresh reload.
//
// So a lost reload is treated as what it is — a step that needs repeating — rather than a
// failure to report to the client.
func (service *Service) loadModelConfig(runtime *backendRuntime, ctx context.Context, modelID string, configFilename string, readiness backendReadiness) error {
	var lastErr error
	for attempt := 1; attempt <= modelLoadAttempts; attempt++ {
		watch := service.watchBackendReadiness(runtime, readiness)
		err := service.reloadModelConfig(runtime, ctx, modelID, configFilename)
		if err == nil {
			err = service.waitForBackendEndpointWatching(runtime, ctx, readiness, modelID, configFilename, watch)
		}
		watch.close()
		if err == nil {
			return nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return err
		}
		// Only a lost reload is worth repeating. Anything else — a dead process, a
		// capability the config does not enable — repeats the same failure and multiplies
		// the work, so it is returned as-is.
		if !errors.Is(err, errBackendServingNoModel) {
			return err
		}
		if attempt < modelLoadAttempts {
			service.logger.Printf("backend did not come up with the model; reissuing config load model=%q config=%q attempt=%d error=%v", modelID, configFilename, attempt, err)
		}
	}
	return lastErr
}

// reloadHeldModelConfig reloads the config the caller already holds a lease on, without
// ever giving the runtime up.
//
// Recovery used to release the lease and re-acquire. Dropping to zero users is the signal
// other waiters are blocked on, so a request wanting a different model would take the
// runtime in that gap and switch it — severing the in-flight request this recovery was
// trying to rescue, and leaving the two models trading the runtime. Holding the lease
// keeps competitors waiting: the caller waits until it is the *only* user rather than
// until there are none.
func (service *Service) reloadHeldModelConfig(runtime *backendRuntime, ctx context.Context, modelID string, configFilename string, readiness backendReadiness) error {
	state := runtime.state
	profile := service.chatTemplateProfileForConfig(configFilename)
	state.mu.Lock()
	ticket := state.takeTicketLocked()
	state.joinQueueLocked(ticket, neverSatisfied)
	for {
		// Our own lease is the one user we expect; anything more means another request is
		// still on the backend and must finish first.
		if state.switching || state.users > 1 {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				leaveSwitchQueue(state, ticket)
				return err
			}
			state.mu.Lock()
			continue
		}
		state.leaveQueueLocked(ticket)
		beginRuntimeSwitchLocked(state, contextIsBorrowed(ctx))
		state.pendingFilename = configFilename
		state.pendingProfile = profile
		state.mu.Unlock()

		loadMeasurement := service.beginModelLoad(ctx)
		err := service.loadModelConfig(runtime, ctx, modelID, configFilename, readiness)
		service.finishModelLoad(ctx, loadMeasurement)

		state.mu.Lock()
		endRuntimeSwitchLocked(state)
		state.pendingFilename = ""
		state.pendingProfile = catalog.ChatTemplateProfile{}
		if err != nil {
			state.filename = ""
			state.modelID = ""
			state.physicalAttemptID = ""
			clearPhysicalLoadProfileLocked(state)
			clearLoadMeasurementLocked(state)
			notifyActiveConfigLocked(state)
			state.mu.Unlock()
			service.onRuntimeChanged()
			return err
		}
		state.filename = configFilename
		state.modelID = modelID
		state.generation++
		applyPhysicalLoadProfileLocked(state, configFilename, profile, readiness)
		applyLoadMeasurementLocked(state, loadMeasurement)
		state.openLeaseWindowLocked()
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		service.analytics.recordLoad(modelID, configFilename, readiness, runtime.mode, loadMeasurement.analytics)
		service.onRuntimeChanged()
		return nil
	}
}

func (service *Service) acquireModelConfig(runtime *backendRuntime, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool) (func(), bool, error) {
	return service.acquireModelConfigWithOptions(runtime, ctx, modelID, configFilename, readiness, modelConfigAcquireOptions{forceReload: force})
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

func (service *Service) backendLoadDiagnosticError(err error, runtime *backendRuntime, modelID string, configFilename string, finish func(bool) backenddiagnostic.Diagnostic) error {
	diagnostic := finish(false)
	diagnostic.NodeID = service.nodeID
	diagnostic.Backend = runtime.name
	wrapped := backenddiagnostic.WithDiagnostic(err, diagnostic)
	service.recordLoadError(loaderrors.RecordInput{
		Phase:       loaderrors.PhasePreload,
		Severity:    loaderrors.SeverityError,
		Source:      "proxy.backendLoadDiagnosticError",
		ModelID:     modelID,
		ConfigName:  configFilename,
		Backend:     runtime.name,
		BackendMode: runtime.mode,
		Message:     err.Error(),
		Output:      diagnostic.Output,
		ExitError:   diagnostic.ExitError,
		Truncated:   diagnostic.Truncated,
	})
	return wrapped
}

var (
	errRuntimeGenerationChanged   = errors.New("runtime changed before unload")
	errRuntimeNoLongerHoldsConfig = errors.New("runtime no longer holds the config to unload")
)

func (service *Service) unloadRuntime(ctx context.Context, runtime *backendRuntime) error {
	return service.unloadRuntimeWhile(ctx, runtime, runtimeAlwaysUnloadable)
}

func (service *Service) unloadRuntimeIfGeneration(ctx context.Context, runtime *backendRuntime, expectedGeneration uint64) error {
	return service.unloadRuntimeWhile(ctx, runtime, func(state *activeConfigState) error {
		if state.generation != expectedGeneration || state.modelID == "" {
			return errRuntimeGenerationChanged
		}
		return ctx.Err()
	})
}

func (service *Service) unloadRuntimeWhile(ctx context.Context, runtime *backendRuntime, stillUnloadable func(*activeConfigState) error) error {
	if err := claimIdleRuntime(ctx, runtime.state, stillUnloadable); err != nil {
		return err
	}
	err := runtime.backend.Unload(ctx)
	finishRuntimeSwitch(runtime.state)
	service.onRuntimeChanged()
	return err
}

func lockRuntimeForBackendStop(ctx context.Context, runtime *backendRuntime) (func(), error) {
	if runtime == nil {
		return func() {}, nil
	}
	if err := claimIdleRuntime(ctx, runtime.state, runtimeAlwaysUnloadable); err != nil {
		return nil, err
	}
	return func() { finishRuntimeSwitch(runtime.state) }, nil
}

func runtimeAlwaysUnloadable(*activeConfigState) error {
	return nil
}

func claimIdleRuntime(ctx context.Context, state *activeConfigState, stillUnloadable func(*activeConfigState) error) error {
	state.mu.Lock()
	ticket := state.takeTicketLocked()
	state.joinQueueLocked(ticket, neverSatisfied)
	for {
		if err := stillUnloadable(state); err != nil {
			state.leaveQueueLocked(ticket)
			state.mu.Unlock()
			return err
		}
		if state.switching || state.users > 0 || !state.maySwitchLocked(ticket) {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				leaveSwitchQueue(state, ticket)
				return err
			}
			state.mu.Lock()
			continue
		}

		state.modelID = ""
		state.generation++
		state.leaveQueueLocked(ticket)
		beginRuntimeSwitchLocked(state, false)
		state.filename = ""
		clearPhysicalLoadProfileLocked(state)
		clearLoadMeasurementLocked(state)
		notifyActiveConfigLocked(state)
		state.mu.Unlock()
		return nil
	}
}

func finishRuntimeSwitch(state *activeConfigState) {
	state.mu.Lock()
	endRuntimeSwitchLocked(state)
	notifyActiveConfigLocked(state)
	state.mu.Unlock()
}

func releaseActiveConfigOnce(state *activeConfigState) func() {
	return releaseActiveConfigLeaseOnce(state, 0, false)
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

func applyPhysicalLoadProfileLocked(state *activeConfigState, filename string, profile catalog.ChatTemplateProfile, readiness backendReadiness) {
	state.physicalFilename = filename
	state.physicalReadiness = readiness
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
		service.recordLoadError(loaderrors.RecordInput{
			Phase:    loaderrors.PhaseConfigParse,
			Severity: loaderrors.SeverityWarning,
			Source:   "proxy.chatTemplateProfileForConfig",
			Message:  "catalog list failed while resolving the chat-template profile: " + err.Error(),
		})
		return catalog.ChatTemplateProfile{}
	}
	for _, model := range models {
		if model.Filename == filename {
			return model.ChatTemplate
		}
	}
	return catalog.ChatTemplateProfile{}
}

func (service *Service) readinessForConfig(filename string, readiness backendReadiness) (backendReadiness, error) {
	if readiness != readinessEmbeddings || filename == "" {
		return readiness, nil
	}
	if filename != filepath.Base(filename) {
		return readiness, fmt.Errorf("config filename %q is invalid", filename)
	}
	if service.catalog != nil {
		separate, found, err := service.catalogConfigSeparatesEmbeddings(filename)
		if err != nil {
			return readiness, err
		}
		if found {
			return embeddingsReadiness(separate), nil
		}
	}
	if service.configDir == "" {
		return readinessText, nil
	}
	metadata, err := catalog.LoadRuntimeConfig(filepath.Join(service.configDir, filename))
	if os.IsNotExist(err) {
		return readinessText, nil
	}
	if err != nil {
		return readiness, err
	}
	return embeddingsReadiness(metadata.RunEmbedSeparate), nil
}

func (service *Service) catalogConfigSeparatesEmbeddings(filename string) (bool, bool, error) {
	models, err := service.catalog.List()
	if err != nil {
		return false, false, err
	}
	for _, model := range models {
		if model.Filename == filename {
			return model.Capabilities.Embeddings != nil && model.Capabilities.Embeddings.Separate, true, nil
		}
	}
	return false, false, nil
}

func embeddingsReadiness(separate bool) backendReadiness {
	if separate {
		return readinessEmbeddings
	}
	return readinessText
}

func releaseActiveConfigLeaseOnce(state *activeConfigState, leaseTag uint64, borrowed bool) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			state.mu.Lock()
			defer state.mu.Unlock()
			if leaseTag != 0 {
				delete(state.leases, leaseTag)
			}
			if state.users > 0 {
				releaseActiveConfigUserLocked(state, borrowed)
			}
		})
	}
}

func releaseActiveConfigUserLocked(state *activeConfigState, borrowed bool) {
	state.users--
	if borrowed && state.borrowedUsers > 0 {
		state.borrowedUsers--
	}
	if !borrowed && state.users == state.borrowedUsers {
		state.ownIdleSince = time.Now()
	}
	if state.users == 0 {
		notifyActiveConfigLocked(state)
	}
}
