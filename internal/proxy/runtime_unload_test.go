package proxy

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"tensors-router/internal/siteapi"
)

func TestDisabledModelUnloadStopsReportingTheModelAsLoaded(t *testing.T) {
	service, _, _ := newModelStateTestService(t)
	runtime := defaultFamilyRuntime(t, service, readinessText)
	backend := runtime.backend.(*fakeBackend)
	state := runtime.state
	state.mu.Lock()
	state.filename = "model-a.kcpps"
	state.modelID = "model-a"
	state.generation = 1
	state.mu.Unlock()

	disabled := postModelState(service, "/router/v1/site/models/state", siteapi.ModelStateRequest{NodeID: "node-a", LocalID: "model-a", Enabled: false}, "")
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	waitForUnloadCount(t, backend, 1)

	state.mu.Lock()
	modelID, generation := state.modelID, state.generation
	state.mu.Unlock()
	if modelID != "" {
		t.Fatalf("unloaded runtime still reports model %q as loaded", modelID)
	}
	if generation == 1 {
		t.Fatal("unload kept the loaded generation, so a stale node unload would still match it")
	}
}

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

	unloadByGeneration := func() {
		runtime.state.mu.Lock()
		generation := runtime.state.generation
		runtime.state.mu.Unlock()
		if err := service.unloadRuntimeIfGeneration(ctx, runtime, generation); err != nil && !errors.Is(err, errRuntimeGenerationChanged) {
			t.Error(err)
		}
	}
	unloadDisabled := func() {
		if err := service.unloadDisabledRuntime(ctx, runtime, "model-a", "model-a.kcpps"); err != nil {
			t.Error(err)
		}
	}

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
	var unloaders sync.WaitGroup
	for _, unload := range []func(){unloadByGeneration, unloadDisabled} {
		unloaders.Go(func() {
			for {
				select {
				case <-acquiring:
					return
				default:
					unload()
				}
			}
		})
	}
	unloaders.Wait()

	if unloadedUnderLease.Load() {
		t.Fatal("backend unloaded while a request still held the runtime")
	}
	if backend.unloads.Load() == 0 {
		t.Fatal("no unload ran, so the race was never exercised")
	}
}
