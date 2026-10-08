package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"tensors-router/internal/catalog"
)

func TestLoadRejectsInvalidExplicitBackendMode(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"bad": `{"model_param":"text.gguf","backend_mode":"native"}`,
	})

	err := service.loadLocalModel(context.Background(), "bad", "bad")
	if err == nil || !strings.Contains(err.Error(), "backend_mode") {
		t.Fatalf("expected backend_mode validation error, got %v", err)
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("invalid backend_mode should not reload backend, got %d", backend.reloads.Load())
	}
}

func TestLoadSwitchesBackendFamiliesBeforeReload(t *testing.T) {
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "kobold", `{"model_param":"kobold.gguf","backend_mode":"kobold"}`)
	writeProxyTestConfig(t, dir, "native", `{"model_param":"native.gguf","backend_mode":"llama_sdcpp"}`)

	var mu sync.Mutex
	events := []string{}
	record := func(event string) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}
	koboldBackend := newReadyFakeBackend(t, func(string) { record("reload-kobold") })
	llamaBackend := newReadyFakeBackend(t, func(string) { record("reload-native") })
	sdcppBackend := newReadyFakeBackend(t, nil)
	service := NewService(ServiceConfig{
		BackendMode: BackendModeKobold,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeKobold: {
				TextBackend:  koboldBackend,
				ImageBackend: koboldBackend,
				Stop: func(context.Context) error {
					record("stop-kobold")
					return nil
				},
			},
			BackendModeLlamaSDCPP: {
				TextBackend:  llamaBackend,
				ImageBackend: sdcppBackend,
				Stop: func(context.Context) error {
					record("stop-native")
					return nil
				},
			},
		},
		Catalog: catalog.New(dir),
		Logger:  log.New(io.Discard, "", 0),
	})

	if err := service.loadLocalModel(context.Background(), "native", "native"); err != nil {
		t.Fatal(err)
	}
	if !eventBefore(eventsSnapshot(&mu, &events), "stop-kobold", "reload-native") {
		t.Fatalf("kobold was not stopped before native reload: %#v", eventsSnapshot(&mu, &events))
	}
	if koboldBackend.unloads.Load() != 0 {
		t.Fatalf("kobold switch-away should use Stop, got unloads=%d", koboldBackend.unloads.Load())
	}

	if err := service.loadLocalModel(context.Background(), "kobold", "kobold"); err != nil {
		t.Fatal(err)
	}
	if !eventBefore(eventsSnapshot(&mu, &events), "stop-native", "reload-kobold") {
		t.Fatalf("native was not stopped before kobold reload: %#v", eventsSnapshot(&mu, &events))
	}
}

func TestBackendFamilySwitchWaitsForInFlightConfigUsers(t *testing.T) {
	dir := t.TempDir()
	writeProxyTestConfig(t, dir, "kobold", `{"model_param":"kobold.gguf","backend_mode":"kobold"}`)
	writeProxyTestConfig(t, dir, "native", `{"model_param":"native.gguf","backend_mode":"llama_sdcpp"}`)

	nativeReloaded := make(chan struct{}, 1)
	service := NewService(ServiceConfig{
		BackendMode: BackendModeKobold,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeKobold: {
				TextBackend:  newReadyFakeBackend(t, nil),
				ImageBackend: newReadyFakeBackend(t, nil),
			},
			BackendModeLlamaSDCPP: {
				TextBackend: newReadyFakeBackend(t, func(string) {
					nativeReloaded <- struct{}{}
				}),
				ImageBackend: newReadyFakeBackend(t, nil),
			},
		},
		Catalog: catalog.New(dir),
		Logger:  log.New(io.Discard, "", 0),
	})
	_, release, _, err := service.acquireModelConfigForBackendMode(BackendModeKobold, context.Background(), "kobold", "kobold.kcpps", readinessText, false)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- service.loadLocalModel(context.Background(), "native", "native")
	}()
	select {
	case <-nativeReloaded:
		t.Fatalf("backend family switched while config user was active")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatalf("backend family switch did not finish after active user release")
	}
}

func TestPreloadModelLoadsAndReusesConfig(t *testing.T) {
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"content":"ready"}}]}`))
	}))

	if err := service.PreloadModel(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one preload reload, got %d", backend.reloads.Load())
	}
	if service.currentConfigFilename() != "a.kcpps" {
		t.Fatalf("unexpected active config %q", service.currentConfigFilename())
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected preloaded config to be reused, got %d reloads", backend.reloads.Load())
	}
}

func TestPreloadModelPreservesCanonicalConfigAndModelPathCase(t *testing.T) {
	mixedModelPath := filepath.Join(t.TempDir(), "MixedCase", "Models", "GemmaModel.GGUF")
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}), map[string]string{
		"Gemma4-31B-NoThink": fmt.Sprintf(`{"model_param":%q}`, mixedModelPath),
	})

	if err := service.PreloadModel(context.Background(), "Gemma4-31B-NoThink"); err != nil {
		t.Fatal(err)
	}
	if backend.lastReload != "Gemma4-31B-NoThink.kcpps" {
		t.Fatalf("reload lost canonical config filename: %q", backend.lastReload)
	}
	model, ok, err := service.catalog.Resolve("Gemma4-31B-NoThink")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || string(model.Options["model_param"]) != fmt.Sprintf("%q", mixedModelPath) {
		t.Fatalf("model path casing was not preserved: %#v", model.Options)
	}
}

func TestPreloadModelRejectsInvalidModel(t *testing.T) {
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	err := service.PreloadModel(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected missing model error")
	}
	if !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("unexpected error %v", err)
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("expected no reload, got %d", backend.reloads.Load())
	}
}

func TestPreloadModelRejectsImageOnlyModel(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})

	err := service.PreloadModel(context.Background(), "image")
	if err == nil {
		t.Fatalf("expected image-only model error")
	}
	if !strings.Contains(err.Error(), "is not a text-lane model") {
		t.Fatalf("unexpected error %v", err)
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("expected no reload, got %d", backend.reloads.Load())
	}
}

func TestRouterLoadRequestUnloadPolicyDoesNotUnloadLanes(t *testing.T) {
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(
		t,
		readyTextHandler(t),
		readyImageHandler(t),
		map[string]string{
			"text":  `{"model_param":"C:\\models\\text.gguf"}`,
			"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
			"next":  `{"model_param":"C:\\models\\next.gguf"}`,
		},
	)

	postRouterLoad(t, service, `{"model":"text"}`)
	postRouterLoad(t, service, `{"model":"image"}`)
	postRouterLoad(t, service, `{"model":"next","unload_policy":"all"}`)

	if textBackend.unloads.Load() != 0 || imageBackend.unloads.Load() != 0 {
		t.Fatalf("request unload_policy should not unload lanes, got text=%d image=%d", textBackend.unloads.Load(), imageBackend.unloads.Load())
	}
	if textBackend.lastReload != "next.kcpps" {
		t.Fatalf("unexpected final text reload %q", textBackend.lastReload)
	}
}

func TestConfigUnloadPolicyAppliesToRouterLoad(t *testing.T) {
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(
		t,
		readyTextHandler(t),
		readyImageHandler(t),
		map[string]string{
			"image":  `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
			"strict": `{"model_param":"C:\\models\\strict.gguf","router_unload_policy":"image"}`,
		},
	)

	postRouterLoad(t, service, `{"model":"image"}`)
	postRouterLoad(t, service, `{"model":"strict"}`)

	if textBackend.unloads.Load() != 0 || imageBackend.unloads.Load() != 1 {
		t.Fatalf("expected config policy to unload image lane, got text=%d image=%d", textBackend.unloads.Load(), imageBackend.unloads.Load())
	}
	if textBackend.lastReload != "strict.kcpps" {
		t.Fatalf("unexpected final text reload %q", textBackend.lastReload)
	}
}

func TestCombinedConfigUnloadPolicyAllKeepsSameFilenameLanes(t *testing.T) {
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(
		t,
		readyTextHandler(t),
		readyImageHandler(t),
		map[string]string{
			"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\dream.safetensors","router_unload_policy":"all"}`,
		},
	)

	postRouterLoad(t, service, `{"model":"combo"}`)

	if textBackend.unloads.Load() != 0 || imageBackend.unloads.Load() != 0 {
		t.Fatalf("same config lanes should not unload, got text=%d image=%d", textBackend.unloads.Load(), imageBackend.unloads.Load())
	}
	if textBackend.reloads.Load() != 1 || imageBackend.reloads.Load() != 1 {
		t.Fatalf("expected both lanes to load, got text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}

func TestKoboldUnloadTargetCollapsesToSharedRuntime(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, readyTextHandler(t), map[string]string{
		"text": `{"model_param":"C:\\models\\text.gguf"}`,
	})

	postRouterLoad(t, service, `{"model":"text"}`)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/unload", strings.NewReader(`{"target":"image"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.unloads.Load() != 1 {
		t.Fatalf("expected shared runtime unload, got %d", backend.unloads.Load())
	}
}

func TestRouterUnloadCallsBackendUnload(t *testing.T) {
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/unload", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.unloads.Load() != 1 {
		t.Fatalf("expected one unload, got %d", backend.unloads.Load())
	}
}

func TestRouterUnloadWaitsForStreamingRequest(t *testing.T) {
	streamStarted := make(chan struct{})
	releaseStream := make(chan struct{})
	unloaded := make(chan struct{})
	var unloadOnce sync.Once
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"model\":\"backend\",\"choices\":[]}\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(streamStarted)
			<-releaseStream
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case koboldPerfPath:
			http.NotFound(w, r)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}), map[string]string{
		"llm": `{}`,
	})
	backend.onUnload = func() {
		unloadOnce.Do(func() {
			close(unloaded)
		})
	}

	streamDone := make(chan struct{})
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[],"stream":true}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		close(streamDone)
	}()

	select {
	case <-streamStarted:
	case <-time.After(time.Second):
		t.Fatal("stream did not start")
	}

	unloadDone := make(chan struct{})
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/router/v1/unload", nil)
		service.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("unexpected unload status %d body %s", recorder.Code, recorder.Body.String())
		}
		close(unloadDone)
	}()

	select {
	case <-unloaded:
		t.Fatal("backend unloaded while stream was active")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseStream)

	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("stream did not finish")
	}
	select {
	case <-unloadDone:
	case <-time.After(time.Second):
		t.Fatal("unload did not finish")
	}
	if backend.unloads.Load() != 1 {
		t.Fatalf("expected one unload, got %d", backend.unloads.Load())
	}
}

func TestRouterShutdownRequiresConfiguredHook(t *testing.T) {
	service := NewService(ServiceConfig{})
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/shutdown", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected shutdown status %d body %s", recorder.Code, recorder.Body.String())
	}
}

func TestRouterShutdownTriggersConfiguredHook(t *testing.T) {
	triggered := make(chan struct{})
	var once sync.Once
	service := NewService(ServiceConfig{
		Shutdown: func() {
			once.Do(func() {
				close(triggered)
			})
		},
	})
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/shutdown", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected shutdown status %d body %s", recorder.Code, recorder.Body.String())
	}
	select {
	case <-triggered:
	case <-time.After(time.Second):
		t.Fatal("shutdown hook was not called")
	}
}

func postRouterLoad(t *testing.T, service *Service, body string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/load", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected load status %d body %s", recorder.Code, recorder.Body.String())
	}
}
