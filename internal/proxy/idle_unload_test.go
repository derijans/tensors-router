package proxy

import (
	"context"
	"io"
	"log"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/catalog"
)

type idleTestClock struct {
	ahead atomic.Int64
}

func (clock *idleTestClock) now() time.Time {
	return time.Now().Add(time.Duration(clock.ahead.Load()))
}

func (clock *idleTestClock) advance(by time.Duration) {
	clock.ahead.Store(int64(by))
}

func newIdleUnloadTestService(t *testing.T, configs map[string]string) (*Service, *fakeBackend, *idleTestClock) {
	t.Helper()
	dir := t.TempDir()
	for id, content := range configs {
		writeProxyTestConfig(t, dir, id, content)
	}
	backend := newReadyFakeBackend(t, nil)
	service := NewService(ServiceConfig{Backend: backend, Catalog: catalog.New(dir), ConfigDir: dir, Logger: log.New(io.Discard, "", 0)})
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	clock := &idleTestClock{}
	service.idleUnloads.now = clock.now
	return service, backend, clock
}

func loadAndRelease(t *testing.T, service *Service, runtime *backendRuntime, modelID string) {
	t.Helper()
	release, _, err := service.acquireModelConfig(runtime, context.Background(), modelID, modelID+".kcpps", readinessText, false)
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestIdleUnloadUnloadsOnlyAfterTheConfiguredDelay(t *testing.T) {
	service, backend, clock := newIdleUnloadTestService(t, map[string]string{"model-a": `{"router_idle_unload_seconds":30}`})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, "model-a")

	clock.advance(20 * time.Second)
	service.idleUnloads.sweep(context.Background())
	if backend.unloads.Load() != 0 {
		t.Fatal("unloaded before the idle delay elapsed")
	}

	clock.advance(31 * time.Second)
	service.idleUnloads.sweep(context.Background())
	if backend.unloads.Load() != 1 {
		t.Fatalf("unloads=%d, want 1 after the idle delay", backend.unloads.Load())
	}
	if _, filename := runtime.state.loadedModel(); filename != "" {
		t.Fatalf("runtime still reports %q loaded after idle unload", filename)
	}
}

func TestIdleUnloadLeavesConfigsWithoutTheKeyLoaded(t *testing.T) {
	service, backend, clock := newIdleUnloadTestService(t, map[string]string{"model-a": `{}`})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, "model-a")

	clock.advance(30 * 24 * time.Hour)
	service.idleUnloads.sweep(context.Background())
	if backend.unloads.Load() != 0 {
		t.Fatal("a config without router_idle_unload_seconds was unloaded")
	}
}

func TestIdleUnloadIgnoresAnInvalidDelay(t *testing.T) {
	service, backend, clock := newIdleUnloadTestService(t, map[string]string{"model-a": `{"router_idle_unload_seconds":"soon"}`})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, "model-a")

	clock.advance(time.Hour)
	service.idleUnloads.sweep(context.Background())
	if backend.unloads.Load() != 0 {
		t.Fatal("an invalid delay must disable idle unload, not unload at once")
	}
}

func TestIdleUnloadNeverUnloadsARuntimeHoldingALease(t *testing.T) {
	service, backend, clock := newIdleUnloadTestService(t, map[string]string{"model-a": `{"router_idle_unload_seconds":1}`})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	release, _, err := service.acquireModelConfig(runtime, context.Background(), "model-a", "model-a.kcpps", readinessText, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	clock.advance(time.Hour)
	service.idleUnloads.sweep(context.Background())
	if backend.unloads.Load() != 0 {
		t.Fatal("idle unload ran while a request held the runtime")
	}
}

func TestIdleUnloadClaimFailsFastInsteadOfWaitingForBusyRuntime(t *testing.T) {
	service, backend, _ := newIdleUnloadTestService(t, map[string]string{"model-a": `{"router_idle_unload_seconds":1}`})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, "model-a")
	snapshot, idle := idleRuntimeSnapshotOf(runtime.state)
	if !idle {
		t.Fatal("runtime should be idle after release")
	}
	release, _, err := service.acquireModelConfig(runtime, context.Background(), "model-a", "model-a.kcpps", readinessText, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = service.unloadRuntimeWhile(ctx, runtime, idleUnloadStillDue(snapshot, time.Second, time.Now().Add(time.Hour)))
	if err != errIdleUnloadBusy {
		t.Fatalf("err=%v, want errIdleUnloadBusy without waiting for the lease", err)
	}
	if backend.unloads.Load() != 0 {
		t.Fatal("busy runtime was unloaded")
	}
}

func TestIdleUnloadSkipsARuntimeThatSwitchedConfigSinceItWentIdle(t *testing.T) {
	service, backend, _ := newIdleUnloadTestService(t, map[string]string{
		"model-a": `{"router_idle_unload_seconds":1}`,
		"model-b": `{"model_param":"b.gguf"}`,
	})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, "model-a")
	snapshot, _ := idleRuntimeSnapshotOf(runtime.state)
	loadAndRelease(t, service, runtime, "model-b")
	unloadsBefore := backend.unloads.Load()

	err := service.unloadRuntimeWhile(context.Background(), runtime, idleUnloadStillDue(snapshot, time.Second, time.Now().Add(time.Hour)))
	if err != errIdleUnloadStale {
		t.Fatalf("err=%v, want errIdleUnloadStale", err)
	}
	if backend.unloads.Load() != unloadsBefore {
		t.Fatal("the config loaded after the idle snapshot was unloaded")
	}
}

func TestIdleUnloadWaitsWhileABorrowRestoreIsPending(t *testing.T) {
	service, backend, clock := newIdleUnloadTestService(t, map[string]string{"model-a": `{"router_idle_unload_seconds":1}`})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, "model-a")
	service.scheduler.recordFirstDisplacement(runtime, runtime.mode, "owner", "owner.kcpps", readinessText)

	clock.advance(time.Hour)
	service.idleUnloads.sweep(context.Background())
	if backend.unloads.Load() != 0 {
		t.Fatal("idle unload fought a pending borrow restore")
	}
}

func TestIdleUnloadCoversSeparatePoolRuntimes(t *testing.T) {
	service, _, separates := newSeparatePoolTestService(t, 5, map[string]string{
		"sep": `{"backend_mode":"kobold","nomodel":true,"model":[],"embeddingsmodel":"C:/models/embed.gguf","run_embed_separate":true,"router_idle_unload_seconds":5}`,
	})
	clock := &idleTestClock{}
	service.idleUnloads.now = clock.now
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	postProxyModelRequest(t, service, "/v1/embeddings", `{"model":"sep","input":"x"}`)

	clock.advance(time.Minute)
	service.idleUnloads.sweep(context.Background())
	if separates["sep"].unloads.Load() != 1 {
		t.Fatalf("separate runtime unloads=%d, want 1", separates["sep"].unloads.Load())
	}
}

func TestIdleUnloadLoopStopsOnClose(t *testing.T) {
	service, backend, clock := newIdleUnloadTestService(t, map[string]string{"model-a": `{"router_idle_unload_seconds":1}`})
	service.idleUnloads.interval = time.Millisecond
	service.StartIdleUnloads(context.Background())
	if err := service.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	runtime := defaultFamilyRuntime(t, service, readinessText)
	loadAndRelease(t, service, runtime, "model-a")
	clock.advance(time.Hour)
	time.Sleep(50 * time.Millisecond)
	if backend.unloads.Load() != 0 {
		t.Fatal("idle unload loop kept running after Close")
	}
}
