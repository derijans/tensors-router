package proxy

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRuntimeUnloadEntryPointsNeverUnloadUnderALease(t *testing.T) {
	service, store, _ := newModelStateTestService(t)
	runtime := defaultFamilyRuntime(t, service, readinessText)
	backend := runtime.backend.(*fakeBackend)
	var unloadedUnderLease atomic.Bool
	backend.onUnload = func() {
		runtime.state.mu.Lock()
		if runtime.state.users > 0 {
			unloadedUnderLease.Store(true)
		}
		runtime.state.mu.Unlock()
	}
	ctx := context.Background()
	if err := store.SetEnabled(ctx, "model-a", false); err != nil {
		t.Fatal(err)
	}

	acquiring := hammerModelAcquisition(t, ctx, service, runtime)
	unloadRepeatedlyUntil(acquiring,
		func() { unloadCurrentGeneration(t, ctx, service, runtime) },
		func() {
			if err := service.unloadDisabledRuntime(ctx, runtime, "model-a", "model-a.kcpps"); err != nil {
				t.Error(err)
			}
		},
	)

	if unloadedUnderLease.Load() {
		t.Fatal("backend unloaded while a request still held the runtime")
	}
	if backend.unloads.Load() == 0 {
		t.Fatal("no unload ran, so the race was never exercised")
	}
}

func hammerModelAcquisition(t *testing.T, ctx context.Context, service *Service, runtime *backendRuntime) <-chan struct{} {
	var acquirers sync.WaitGroup
	for range 4 {
		acquirers.Go(func() {
			for range 25 {
				release, _, err := service.acquireModelConfig(runtime, ctx, "model-a", "model-a.kcpps", readinessText, false)
				if err != nil {
					t.Error(err)
					return
				}
				release()
			}
		})
	}
	acquiring := make(chan struct{})
	go func() {
		acquirers.Wait()
		close(acquiring)
	}()
	return acquiring
}

func unloadRepeatedlyUntil(done <-chan struct{}, unloads ...func()) {
	var unloaders sync.WaitGroup
	for _, unload := range unloads {
		unloaders.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
					unload()
				}
			}
		})
	}
	unloaders.Wait()
}

func unloadCurrentGeneration(t *testing.T, ctx context.Context, service *Service, runtime *backendRuntime) {
	runtime.state.mu.Lock()
	generation := runtime.state.generation
	runtime.state.mu.Unlock()
	if err := service.unloadRuntimeIfGeneration(ctx, runtime, generation); err != nil && !errors.Is(err, errRuntimeGenerationChanged) {
		t.Error(err)
	}
}
