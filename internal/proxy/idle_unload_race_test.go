package proxy

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestIdleUnloadNeverUnloadsUnderALeaseWhileRequestsRace(t *testing.T) {
	service, backend, clock := newIdleUnloadTestService(t, map[string]string{"model-a": `{"router_idle_unload_seconds":1}`})
	runtime := defaultFamilyRuntime(t, service, readinessText)
	var unloadedUnderLease atomic.Bool
	backend.onUnload = func() {
		runtime.state.mu.Lock()
		if runtime.state.users > 0 {
			unloadedUnderLease.Store(true)
		}
		runtime.state.mu.Unlock()
	}
	clock.advance(time.Hour)
	ctx := context.Background()

	acquiring := hammerModelAcquisition(t, ctx, service, runtime)
	unloadRepeatedlyUntil(acquiring, func() { service.idleUnloads.sweep(ctx) })

	if unloadedUnderLease.Load() {
		t.Fatal("idle unload ran while a request still held the runtime")
	}
	if backend.unloads.Load() == 0 {
		t.Fatal("no idle unload ran, so the race was never exercised")
	}
}
