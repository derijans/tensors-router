package proxy

import (
	"context"
	"fmt"
	"sync"

	"tensors-router/internal/backendmode"
	"tensors-router/internal/catalog"
)

type backendFamily struct {
	mode                 string
	textRuntime          *backendRuntime
	embeddingsRuntime    *backendRuntime
	imageRuntime         *backendRuntime
	transcriptionRuntime *backendRuntime
	separateBackend      separateBackendFactory
	start                func(context.Context) error
	stop                 func(context.Context) error
	stopPrimary          func(context.Context) error
}

type backendFamilySwitchState struct {
	mu        sync.Mutex
	changed   chan struct{}
	mode      string
	switching bool
}

func (service *Service) ensureBackendFamily(ctx context.Context, mode string) error {
	resolvedMode, err := service.resolveBackendMode(mode)
	if err != nil {
		return err
	}
	family, ok := service.backendFamilies[resolvedMode]
	if !ok || family == nil {
		return fmt.Errorf("backend mode %q is not configured", resolvedMode)
	}
	state := service.backendSwitch
	for {
		state.mu.Lock()
		if !state.switching && state.mode == resolvedMode {
			state.mu.Unlock()
			return family.startBackend(ctx)
		}
		if state.switching {
			changed := state.changed
			state.mu.Unlock()
			if err := waitForActiveConfigChange(ctx, changed); err != nil {
				return err
			}
			continue
		}
		oldMode := state.mode
		state.switching = true
		state.mu.Unlock()

		nextMode := oldMode
		if oldMode != "" && oldMode != resolvedMode {
			if err := service.stopBackendFamily(ctx, oldMode); err != nil {
				service.finishBackendFamilySwitch(nextMode)
				return err
			}
			nextMode = ""
		}
		if err := family.startBackend(ctx); err != nil {
			service.finishBackendFamilySwitch(nextMode)
			return err
		}
		service.finishBackendFamilySwitch(resolvedMode)
		return nil
	}
}

func (service *Service) finishBackendFamilySwitch(mode string) {
	state := service.backendSwitch
	state.mu.Lock()
	state.mode = mode
	state.switching = false
	close(state.changed)
	state.changed = make(chan struct{})
	state.mu.Unlock()
	service.invalidateWebUIRoutes()
}

func (service *Service) stopBackendFamily(ctx context.Context, mode string) error {
	family := service.backendFamilies[mode]
	if family == nil {
		return nil
	}
	runtimes := uniquePrimaryBackendRuntimes(family)
	releases := make([]func(), 0, len(runtimes))
	for _, runtime := range runtimes {
		release, err := lockRuntimeForBackendStop(ctx, runtime)
		if err != nil {
			for index := len(releases) - 1; index >= 0; index-- {
				releases[index]()
			}
			return err
		}
		releases = append(releases, release)
	}
	err := family.stopPrimaryBackend(ctx)
	for index := len(releases) - 1; index >= 0; index-- {
		releases[index]()
	}
	return err
}

func uniquePrimaryBackendRuntimes(family *backendFamily) []*backendRuntime {
	if family == nil || family.textRuntime == nil {
		return nil
	}
	runtimes := []*backendRuntime{family.textRuntime}
	if family.imageRuntime != nil && family.imageRuntime != family.textRuntime {
		runtimes = append(runtimes, family.imageRuntime)
	}
	if family.transcriptionRuntime != nil && family.transcriptionRuntime != family.textRuntime && family.transcriptionRuntime != family.imageRuntime {
		runtimes = append(runtimes, family.transcriptionRuntime)
	}
	return runtimes
}

func uniqueBackendRuntimes(family *backendFamily) []*backendRuntime {
	if family == nil || family.textRuntime == nil {
		return nil
	}
	runtimes := []*backendRuntime{family.textRuntime}
	if family.imageRuntime != nil && family.imageRuntime != family.textRuntime {
		runtimes = append(runtimes, family.imageRuntime)
	}
	if family.embeddingsRuntime != nil && family.embeddingsRuntime != family.textRuntime && family.embeddingsRuntime != family.imageRuntime {
		runtimes = append(runtimes, family.embeddingsRuntime)
	}
	if family.transcriptionRuntime != nil && family.transcriptionRuntime != family.textRuntime && family.transcriptionRuntime != family.imageRuntime {
		runtimes = append(runtimes, family.transcriptionRuntime)
	}
	return runtimes
}

func (family *backendFamily) startBackend(ctx context.Context) error {
	if family.start == nil {
		return nil
	}
	return family.start(ctx)
}

func (family *backendFamily) stopPrimaryBackend(ctx context.Context) error {
	if family.stopPrimary != nil {
		return family.stopPrimary(ctx)
	}
	if family.stop != nil {
		return family.stop(ctx)
	}
	var firstErr error
	for _, runtime := range uniquePrimaryBackendRuntimes(family) {
		if err := runtime.backend.Unload(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (service *Service) resolveBackendMode(mode string) (string, error) {
	return backendmode.Resolve(mode, service.backendMode)
}

func (service *Service) currentBackendMode() string {
	state := service.backendSwitch
	state.mu.Lock()
	mode := state.mode
	state.mu.Unlock()
	if mode == "" {
		mode = service.backendMode
	}
	return mode
}

func (service *Service) runtimeForBackendMode(mode string, readiness backendReadiness) (*backendRuntime, error) {
	resolvedMode, err := service.resolveBackendMode(mode)
	if err != nil {
		return nil, err
	}
	family := service.backendFamilies[resolvedMode]
	if family == nil {
		return nil, fmt.Errorf("backend mode %q is not configured", resolvedMode)
	}
	if readiness == readinessImage {
		return family.imageRuntime, nil
	}
	if readiness == readinessEmbeddings {
		return family.embeddingsRuntime, nil
	}
	if readiness == readinessTranscription {
		return family.transcriptionRuntime, nil
	}
	return family.textRuntime, nil
}

func (service *Service) localBackendAvailableForRoute(ctx context.Context, mode string, readiness backendReadiness) bool {
	resolvedMode, err := service.resolveBackendMode(mode)
	if err != nil {
		return false
	}
	runtime, err := service.runtimeForBackendMode(resolvedMode, readiness)
	if err != nil || runtime == nil {
		return false
	}
	if service.currentBackendMode() != resolvedMode {
		return true
	}
	if localRuntimeAcceptsQueuedRequest(runtime) {
		return true
	}
	return runtime.backend.Healthy(ctx)
}

func localRuntimeAcceptsQueuedRequest(runtime *backendRuntime) bool {
	runtime.state.mu.Lock()
	defer runtime.state.mu.Unlock()
	return runtime.state.filename == "" || runtime.state.switching || runtime.state.switchWaiters > 0
}

func (service *Service) currentConfigFilename() string {
	runtime, err := service.runtimeForBackendMode(service.currentBackendMode(), readinessText)
	if err != nil || runtime == nil {
		return ""
	}
	return currentRuntimeConfigFilename(runtime)
}

func (service *Service) currentImageConfigFilename() string {
	runtime, err := service.runtimeForBackendMode(service.currentBackendMode(), readinessImage)
	if err != nil || runtime == nil {
		return ""
	}
	return currentRuntimeConfigFilename(runtime)
}

func (service *Service) imageCatalogConfigSelector() string {
	if service.currentBackendMode() == BackendModeLlamaSDCPP {
		return catalog.AllImageConfigs
	}
	return service.currentImageConfigFilename()
}
