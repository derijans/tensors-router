package proxy

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMixedArrivalsDuringALoadNeedOnlyOneSwitchPerModel(t *testing.T) {
	service, backend := newTestServiceWithModels(t, http.NotFoundHandler(), "smol", "qwen")
	runtime := defaultFamilyRuntime(t, service, readinessText)
	var loadsMu sync.Mutex
	var loads []string
	backend.onReload = func(filename string) {
		loadsMu.Lock()
		loads = append(loads, filename)
		loadsMu.Unlock()
		time.Sleep(150 * time.Millisecond)
	}

	var requests sync.WaitGroup
	for _, filename := range []string{"smol.kcpps", "qwen.kcpps", "smol.kcpps", "qwen.kcpps", "smol.kcpps"} {
		requests.Go(func() { holdModelLease(t, service, runtime, filename, 30*time.Millisecond) })
		time.Sleep(20 * time.Millisecond)
	}
	requests.Wait()

	loadsMu.Lock()
	defer loadsMu.Unlock()
	if len(loads) != 2 {
		t.Fatalf("3 smol and 2 qwen requests needed %d loads %v, want 2", len(loads), loads)
	}
}

func TestSwitchWaiterIsNotStarvedByAStreamOfLoadedModelRequests(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.NotFoundHandler(), "smol", "qwen")
	runtime := defaultFamilyRuntime(t, service, readinessText)
	holdModelLease(t, service, runtime, "smol.kcpps", 0)

	var stop atomic.Bool
	var stream sync.WaitGroup
	for range 4 {
		stream.Go(func() {
			for !stop.Load() {
				holdModelLease(t, service, runtime, "smol.kcpps", 5*time.Millisecond)
			}
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	release, _, err := service.acquireModelConfig(runtime, ctx, "qwen", "qwen.kcpps", readinessText, false)
	stop.Store(true)
	if err == nil {
		release()
	}
	stream.Wait()
	if err != nil {
		t.Fatalf("qwen waiter starved behind the smol stream: %v", err)
	}
}

func TestHeldLeaseReloadDoesNotDeadlockWithAWaitingSwitch(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.NotFoundHandler(), "smol", "qwen")
	runtime := defaultFamilyRuntime(t, service, readinessText)
	ctx := context.Background()
	release, _, err := service.acquireModelConfig(runtime, ctx, "smol", "smol.kcpps", readinessText, false)
	if err != nil {
		t.Fatal(err)
	}
	waiterDone := make(chan error, 1)
	go func() {
		qwenRelease, _, err := service.acquireModelConfig(runtime, ctx, "qwen", "qwen.kcpps", readinessText, false)
		if err == nil {
			qwenRelease()
		}
		waiterDone <- err
	}()
	time.Sleep(50 * time.Millisecond)

	reloadContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	reloadErr := service.reloadHeldModelConfig(runtime, reloadContext, "smol", "smol.kcpps", readinessText)
	release()

	if reloadErr != nil {
		t.Fatalf("reload of a held model waited on a switch that waits on it: %v", reloadErr)
	}
	if err := <-waiterDone; err != nil {
		t.Fatalf("waiting switch failed after the reload: %v", err)
	}
}

func holdModelLease(t *testing.T, service *Service, runtime *backendRuntime, filename string, hold time.Duration) {
	t.Helper()
	modelID := filename[:len(filename)-len(".kcpps")]
	release, _, err := service.acquireModelConfig(runtime, context.Background(), modelID, filename, readinessText, false)
	if err != nil {
		t.Error(err)
		return
	}
	time.Sleep(hold)
	release()
}
