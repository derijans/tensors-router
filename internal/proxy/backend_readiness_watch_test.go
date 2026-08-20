package proxy

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/loadcapture"
)

// watchableBackend is a fake backend that can emit process output, the way a real
// KoboldCpp or llama.cpp child does.
type watchableBackend struct {
	*fakeBackend
	hub *loadcapture.Hub
}

func (backend *watchableBackend) WatchOutput(observe func(loadcapture.Stream, []byte)) func() {
	return backend.hub.Watch(observe)
}

func (backend *watchableBackend) emit(text string) {
	_, _ = backend.hub.Stdout().Write([]byte(text))
}

// This is the 628-second stall in miniature. KoboldCpp was interrupted mid-load and came
// back serving no model, so /v1/models answers "inactive" forever. The HTTP probe cannot
// tell that apart from a slow load and would burn the entire retry budget; the process
// output says so outright on the first line.
func TestLoadFailsImmediatelyWhenOutputReportsNoModel(t *testing.T) {
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"inactive"}]}`))
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	backend := &watchableBackend{
		fakeBackend: &fakeBackend{url: backendURL, healthy: true},
		hub:         loadcapture.NewHub(),
	}
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "text", `{"model_param":"text.gguf"}`)
	service := NewService(ServiceConfig{
		Backend:   backend,
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(os.Stderr, "", 0),
	})
	service.backendRetryAttempts = 300
	service.backendRetryDelay = 10 * time.Millisecond
	service.backendRetryMaxDelay = 10 * time.Millisecond

	runtime, err := service.runtimeForBackendMode(BackendModeKobold, readinessText)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		backend.emit("Active Modules: AdminControl\nInactive Modules: TextGeneration ImageGeneration\n")
	}()

	started := time.Now()
	err = service.waitForBackendEndpoint(runtime, context.Background(), readinessText, "text", "text.kcpps")
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("expected the load to fail once the backend reported no text model")
	}
	if !strings.Contains(err.Error(), "Inactive Modules") {
		t.Fatalf("error must name the reason from the backend output, got %v", err)
	}
	// The full budget would be 300 attempts; failing fast is the whole point.
	if elapsed > 2*time.Second {
		t.Fatalf("took %s to notice a backend that reported no model", elapsed)
	}
	if probes.Load() > 30 {
		t.Fatalf("probed %d times before believing the backend's own output", probes.Load())
	}
}

// Readiness must NOT be taken from the output. The markers identify a lane, not a model,
// so a banner from one load is visible to the next one's watcher; trusting it declares a
// model loaded that is not, and the request then hits an unloaded backend. Here the probe
// keeps reporting "inactive" while the output claims success — the wait must not finish.
func TestReadyOutputDoesNotShortCircuitTheProbe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"inactive"}]}`))
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	backend := &watchableBackend{
		fakeBackend: &fakeBackend{url: backendURL, healthy: true},
		hub:         loadcapture.NewHub(),
	}
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "text", `{"model_param":"text.gguf"}`)
	service := NewService(ServiceConfig{
		Backend:   backend,
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(os.Stderr, "", 0),
	})
	service.backendRetryAttempts = 4
	service.backendRetryDelay = 10 * time.Millisecond
	service.backendRetryMaxDelay = 10 * time.Millisecond

	runtime, err := service.runtimeForBackendMode(BackendModeKobold, readinessText)
	if err != nil {
		t.Fatal(err)
	}

	backend.emit("Load Text Model OK: True\nActive Modules: TextGeneration AdminControl\n")

	err = service.waitForBackendEndpoint(runtime, context.Background(), readinessText, "text", "text.kcpps")
	if err == nil {
		t.Fatal("a ready banner must not satisfy the wait while the backend still reports inactive")
	}
}

// A backend that becomes genuinely ready is accepted via the probe, with the output
// watcher active and harmless.
func TestLoadSucceedsWhenProbeReportsModelLoaded(t *testing.T) {
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if probes.Add(1) < 2 {
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"inactive"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"koboldcpp/backend"}]}`))
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	backend := &watchableBackend{
		fakeBackend: &fakeBackend{url: backendURL, healthy: true},
		hub:         loadcapture.NewHub(),
	}
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "text", `{"model_param":"text.gguf"}`)
	service := NewService(ServiceConfig{
		Backend:   backend,
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(os.Stderr, "", 0),
	})
	service.backendRetryAttempts = 10
	service.backendRetryDelay = 10 * time.Millisecond
	service.backendRetryMaxDelay = 10 * time.Millisecond

	runtime, err := service.runtimeForBackendMode(BackendModeKobold, readinessText)
	if err != nil {
		t.Fatal(err)
	}
	backend.emit("Load Text Model OK: True\nActive Modules: TextGeneration AdminControl\n")
	if err := service.waitForBackendEndpoint(runtime, context.Background(), readinessText, "text", "text.kcpps"); err != nil {
		t.Fatalf("expected readiness once the probe agrees, got %v", err)
	}
}

// Backends that cannot stream output must still work through the HTTP probe alone.
func TestReadinessFallsBackToProbeWithoutOutput(t *testing.T) {
	var probes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if probes.Add(1) < 3 {
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"inactive"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"koboldcpp/backend"}]}`))
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "text", `{"model_param":"text.gguf"}`)
	service := NewService(ServiceConfig{
		Backend:   &fakeBackend{url: backendURL, healthy: true},
		Catalog:   catalog.New(dir),
		ConfigDir: dir,
		Logger:    log.New(os.Stderr, "", 0),
	})
	service.backendRetryAttempts = 10
	service.backendRetryDelay = 10 * time.Millisecond
	service.backendRetryMaxDelay = 10 * time.Millisecond

	runtime, err := service.runtimeForBackendMode(BackendModeKobold, readinessText)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.waitForBackendEndpoint(runtime, context.Background(), readinessText, "text", "text.kcpps"); err != nil {
		t.Fatalf("probe-only readiness failed: %v", err)
	}
}
