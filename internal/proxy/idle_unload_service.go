package proxy

import (
	"context"
	"time"

	"tensors-router/internal/idleunload"
)

func (service *Service) StartIdleUnloads(ctx context.Context) {
	service.idleUnloads.start(ctx)
}

func (service *Service) idleUnloadTimeout(filename string) (time.Duration, error) {
	if service.catalog == nil {
		return 0, nil
	}
	models, err := service.catalog.List()
	if err != nil {
		return 0, err
	}
	for _, model := range models {
		if model.Filename == filename {
			return idleunload.Timeout(model.Options)
		}
	}
	return 0, nil
}

func (service *Service) borrowRestorePending(runtime *backendRuntime) bool {
	return service.scheduler.borrowRestorePending(runtime)
}

func (scheduler *scheduler) borrowRestorePending(runtime *backendRuntime) bool {
	value, tracked := scheduler.borrowRestore.Load(runtime)
	if !tracked {
		return false
	}
	state := value.(*borrowRestoreState)
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.target != nil
}
