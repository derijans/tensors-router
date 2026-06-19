package proxy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

type fakeBackend struct {
	url        *url.URL
	reloads    atomic.Int32
	restarts   atomic.Int32
	unloads    atomic.Int32
	healthy    bool
	lastReload string
	onReload   func(string)
	onRestart  func()
	onUnload   func()
	reloadErr  func(string) error
	restartErr func() error
}

func (backend *fakeBackend) URL() *url.URL {
	copyValue := *backend.url
	return &copyValue
}

func (backend *fakeBackend) ReloadConfig(ctx context.Context, filename string) error {
	backend.lastReload = filename
	backend.reloads.Add(1)
	if backend.onReload != nil {
		backend.onReload(filename)
	}
	if backend.reloadErr != nil {
		return backend.reloadErr(filename)
	}
	return nil
}

func (backend *fakeBackend) Restart(ctx context.Context) error {
	backend.restarts.Add(1)
	if backend.onRestart != nil {
		backend.onRestart()
	}
	if backend.restartErr != nil {
		return backend.restartErr()
	}
	return nil
}

func (backend *fakeBackend) Unload(ctx context.Context) error {
	backend.unloads.Add(1)
	if backend.onUnload != nil {
		backend.onUnload()
	}
	return nil
}

func (backend *fakeBackend) Healthy(ctx context.Context) bool {
	return backend.healthy
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func testHTTPResponse(status int, contentType string, body string) *http.Response {
	header := http.Header{}
	if contentType != "" {
		header.Set("Content-Type", contentType)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestModelsEndpoint(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `"id":"a"`) {
		t.Fatalf("model list missing a: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), `"id":"a.kcpps"`) {
		t.Fatalf("model list should omit extension: %s", recorder.Body.String())
	}
}

func TestOpenAIBaseEndpointIsForwarded(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"ok":true`) {
		t.Fatalf("base endpoint was not forwarded: %s", recorder.Body.String())
	}
}

func TestAudioSpeechRoutesKnownVoiceConfig(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		content, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `"model":"voice"`) {
			t.Fatalf("request model was not preserved: %s", string(content))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}), map[string]string{
		"voice": `{"ttsmodel":"voice.gguf"}`,
	})
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"voice","input":"hello","voice":"alloy"}`))
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 || backend.lastReload != "voice.kcpps" {
		t.Fatalf("expected voice reload, got count=%d config=%q", backend.reloads.Load(), backend.lastReload)
	}
}

func TestAudioSpeechPassesUnknownModelWithoutConfigReload(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		content, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(content), `"model":"tts-1"`) {
			t.Fatalf("request body changed unexpectedly: %s", string(content))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}), map[string]string{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"tts-1","input":"hello","voice":"alloy"}`))
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("unknown audio model should pass through without reload, got %d", backend.reloads.Load())
	}
}

func TestMusicUIPathPassesThrough(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/musicui" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("music ui"))
	}), map[string]string{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/musicui", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "music ui" {
		t.Fatalf("unexpected musicui response %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSplitModeRejectsAudioRoutes(t *testing.T) {
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("text backend should not receive split audio route")
	}), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("image backend should not receive split audio route")
	}), map[string]string{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(`{"model":"voice","input":"hello"}`))
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if textBackend.reloads.Load() != 0 || imageBackend.reloads.Load() != 0 {
		t.Fatalf("split audio should not reload backends text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}

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

func TestImageModelsUsePerModelBackendModeVisibility(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"kobold-combo": `{"model_param":"text.gguf","sdmodel":"/models/dream.safetensors","backend_mode":"kobold"}`,
		"native-combo": `{"model_param":"text.gguf","sdmodel":"/models/neon.safetensors","backend_mode":"llama_sdcpp"}`,
	})

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "native-combo-neon") {
		t.Fatalf("llama/sd.cpp combined image should be visible: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "kobold-combo-dream") {
		t.Fatalf("inactive kobold combined image should not be visible: %s", recorder.Body.String())
	}
}

func TestUnknownCoreModelReturnsOpenAIError(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"missing"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "missing") {
		t.Fatalf("response missing model name: %s", recorder.Body.String())
	}
}

func TestImageOnlyConfigIsNotCoreModel(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"image","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("expected no reload, got %d", backend.reloads.Load())
	}
}

func TestInvalidModelJSONIsLogged(t *testing.T) {
	var logs bytes.Buffer
	service, _ := newTestServiceWithLogger(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), log.New(&logs, "", 0))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":123}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if !strings.Contains(logs.String(), "model parse failed") {
		t.Fatalf("expected parse log, got %q", logs.String())
	}
}

func TestFallbackReloadsAndRetriesCoreRequest(t *testing.T) {
	var requests atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if requests.Add(1) == 1 {
			http.Error(w, "loading failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","ok":true}`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if backend.lastReload != "a.kcpps" {
		t.Fatalf("unexpected reload config %q", backend.lastReload)
	}
	if !strings.Contains(recorder.Body.String(), `"model":"a"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "koboldcpp/backend") {
		t.Fatalf("backend model leaked: %s", recorder.Body.String())
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two backend requests, got %d", requests.Load())
	}
}

func TestCoreRequestWaitsForBackendModelsEndpointAfterReload(t *testing.T) {
	var probes atomic.Int32
	var chats atomic.Int32
	service, backend := newTestServiceWithRawBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			if probes.Add(1) < 3 {
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(`<h2>KoboldCpp is not available.</h2>`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}

		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		chats.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","ok":true}`))
	}), "a")
	service.backendRetryAttempts = 4
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if probes.Load() != 3 {
		t.Fatalf("expected three readiness probes, got %d", probes.Load())
	}
	if chats.Load() != 1 {
		t.Fatalf("expected one chat request, got %d", chats.Load())
	}
}

func TestCoreRequestWaitsForBackendModelsEndpointUntilNotInactive(t *testing.T) {
	var probes atomic.Int32
	var chats atomic.Int32
	service, backend := newTestServiceWithRawBackend(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			if probes.Add(1) < 3 {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"inactive"}]}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"koboldcpp/backend"}]}`))
			return
		}

		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		chats.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"content":"ready"}}]}`))
	}), "a")
	service.backendRetryAttempts = 4
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if probes.Load() != 3 {
		t.Fatalf("expected three readiness probes, got %d", probes.Load())
	}
	if chats.Load() != 1 {
		t.Fatalf("expected one chat request, got %d", chats.Load())
	}
}

func TestFallbackWaitsAfterInactiveCoreResponse(t *testing.T) {
	var probes atomic.Int32
	var posts atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	service.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
				if probes.Add(1) == 1 {
					return testHTTPResponse(http.StatusOK, "application/json", `{"object":"list","data":[]}`), nil
				}
				if probes.Load() < 4 {
					return testHTTPResponse(http.StatusOK, "application/json", `{"object":"list","data":[{"id":"inactive"}]}`), nil
				}
				return testHTTPResponse(http.StatusOK, "application/json", `{"object":"list","data":[{"id":"koboldcpp/backend"}]}`), nil
			case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
				if posts.Add(1) == 1 {
					return testHTTPResponse(http.StatusOK, "application/json", `{"id":"chatcmpl-test","object":"chat.completion","model":"inactive","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[]},"finish_reason":"error","logprobs":null}]}`), nil
				}
				return testHTTPResponse(http.StatusOK, "application/json", `{"model":"koboldcpp/backend","choices":[{"message":{"content":"ready"}}]}`), nil
			default:
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				return nil, nil
			}
		}),
	}
	service.backendRetryAttempts = 5
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if probes.Load() != 4 {
		t.Fatalf("expected four readiness probes, got %d", probes.Load())
	}
	if posts.Load() != 2 {
		t.Fatalf("expected two chat forwards, got %d", posts.Load())
	}
	if !strings.Contains(recorder.Body.String(), `"model":"a"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestBackendRetryResultRejectsBodylessResponse(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	response := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{},
	}

	result := service.backendRetryResult(response, nil, "/v1/chat/completions")

	if !result.retry {
		t.Fatalf("bodyless response was treated as usable")
	}
	if !errors.Is(result.err, errMissingBackendResponse) {
		t.Fatalf("unexpected error %v", result.err)
	}
}

func TestFreshLoadBackendTransportFailureRecoversThroughRestart(t *testing.T) {
	var posts atomic.Int32
	connectionRefused := errors.New("dial tcp 127.0.0.1:5001: connect: connection refused")
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	backend.reloadErr = func(filename string) error {
		if !backend.healthy {
			return connectionRefused
		}
		return nil
	}
	backend.onRestart = func() {
		backend.healthy = true
	}
	service.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
				return testHTTPResponse(http.StatusOK, "application/json", `{"object":"list","data":[]}`), nil
			case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
				if posts.Add(1) == 1 {
					backend.healthy = false
					return nil, connectionRefused
				}
				return testHTTPResponse(http.StatusOK, "application/json", `{"model":"koboldcpp/backend","choices":[{"message":{"content":"ready"}}]}`), nil
			default:
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				return nil, nil
			}
		}),
	}
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.restarts.Load() != 1 {
		t.Fatalf("expected one restart, got %d", backend.restarts.Load())
	}
	if backend.reloads.Load() != 3 {
		t.Fatalf("expected initial reload, failed recovery reload, and retry reload, got %d", backend.reloads.Load())
	}
	if posts.Load() != 2 {
		t.Fatalf("expected two chat forwards, got %d", posts.Load())
	}
	if !strings.Contains(recorder.Body.String(), `"model":"a"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestActiveConfigBackendTransportFailureRecoversThroughRestart(t *testing.T) {
	var posts atomic.Int32
	connectionRefused := errors.New("dial tcp 127.0.0.1:5001: connect: connection refused")
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	backend.reloadErr = func(filename string) error {
		if !backend.healthy {
			return connectionRefused
		}
		return nil
	}
	backend.onRestart = func() {
		backend.healthy = true
	}
	service.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
				return testHTTPResponse(http.StatusOK, "application/json", `{"object":"list","data":[]}`), nil
			case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
				switch posts.Add(1) {
				case 1, 3:
					return testHTTPResponse(http.StatusOK, "application/json", `{"model":"koboldcpp/backend","choices":[{"message":{"content":"ready"}}]}`), nil
				default:
					return nil, connectionRefused
				}
			default:
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				return nil, nil
			}
		}),
	}
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	firstRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(firstRecorder, firstRequest)
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected first status %d body %s", firstRecorder.Code, firstRecorder.Body.String())
	}

	backend.healthy = false

	secondRecorder := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	secondRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(secondRecorder, secondRequest)

	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected second status %d body %s", secondRecorder.Code, secondRecorder.Body.String())
	}
	if backend.restarts.Load() != 1 {
		t.Fatalf("expected one restart, got %d", backend.restarts.Load())
	}
	if backend.reloads.Load() != 3 {
		t.Fatalf("expected initial reload, failed recovery reload, and retry reload, got %d", backend.reloads.Load())
	}
	if posts.Load() != 3 {
		t.Fatalf("expected three chat forwards, got %d", posts.Load())
	}
}

func TestBackendTransportRecoveryIsBounded(t *testing.T) {
	var posts atomic.Int32
	connectionRefused := errors.New("dial tcp 127.0.0.1:5001: connect: connection refused")
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	backend.reloadErr = func(filename string) error {
		if !backend.healthy {
			return connectionRefused
		}
		return nil
	}
	service.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
				return testHTTPResponse(http.StatusOK, "application/json", `{"object":"list","data":[]}`), nil
			case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
				posts.Add(1)
				backend.healthy = false
				return nil, connectionRefused
			default:
				t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				return nil, nil
			}
		}),
	}
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.restarts.Load() != 1 {
		t.Fatalf("expected one restart, got %d", backend.restarts.Load())
	}
	if posts.Load() != 1 {
		t.Fatalf("expected one chat forward before failed recovery, got %d", posts.Load())
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

func TestPreloadModelAcceptsEmbeddingOnlyModel(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"embed"}]}`))
			return
		}
		t.Fatalf("unexpected path %s", r.URL.Path)
	}), map[string]string{
		"embed": `{"nomodel":true,"embeddingsmodel":"C:\\models\\embed.gguf"}`,
	})

	if err := service.PreloadModel(context.Background(), "embed"); err != nil {
		t.Fatal(err)
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one preload reload, got %d", backend.reloads.Load())
	}
}

func TestFallbackRetriesUntilBackendAnswers(t *testing.T) {
	var requests atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) < 4 {
			http.Error(w, "model warming", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","ok":true}`))
	}))
	service.backendRetryAttempts = 4
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if requests.Load() != 4 {
		t.Fatalf("expected four backend requests, got %d", requests.Load())
	}
}

func TestFallbackRetriesEmptySuccessfulCoreResponse(t *testing.T) {
	var requests atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Content-Type", "application/json")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","ok":true}`))
	}))
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two backend requests, got %d", requests.Load())
	}
	if !strings.Contains(recorder.Body.String(), `"model":"a"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestFallbackRetriesEmptyChatCompletionText(t *testing.T) {
	var requests atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"content":""}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"content":"Hello! How can I help you today?"}}]}`))
	}))
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two backend requests, got %d", requests.Load())
	}
	if !strings.Contains(recorder.Body.String(), "Hello! How can I help you today?") {
		t.Fatalf("response missing generated text: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"model":"a"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestFallbackRetriesEmptyCompletionText(t *testing.T) {
	var requests atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"text":""}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"text":"ready"}]}`))
	}))
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(`{"model":"a","prompt":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two backend requests, got %d", requests.Load())
	}
	if !strings.Contains(recorder.Body.String(), `"text":"ready"`) {
		t.Fatalf("response missing completion text: %s", recorder.Body.String())
	}
}

func TestFallbackRetriesEmptySuccessfulStream(t *testing.T) {
	var requests atomic.Int32
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			return
		}
		_, _ = w.Write([]byte("data: {\"model\":\"koboldcpp/backend\",\"choices\":[]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[],"stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if requests.Load() != 2 {
		t.Fatalf("expected two backend requests, got %d", requests.Load())
	}
	if !strings.Contains(recorder.Body.String(), `"model":"a"`) {
		t.Fatalf("stream model was not rewritten: %q", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "data: [DONE]") {
		t.Fatalf("done event missing: %q", recorder.Body.String())
	}
}

func TestWaitsForKoboldUnavailablePageUntilBackendAnswers(t *testing.T) {
	var requests atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) < 4 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`<h2>KoboldCpp is not available.</h2>`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","ok":true}`))
	}))
	service.backendRetryAttempts = 4
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if backend.restarts.Load() != 0 {
		t.Fatalf("expected no restarts, got %d", backend.restarts.Load())
	}
	if requests.Load() != 4 {
		t.Fatalf("expected four backend requests, got %d", requests.Load())
	}
}

func TestSameModelKoboldUnavailablePageDoesNotForceReload(t *testing.T) {
	var requests atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch requests.Add(1) {
		case 1, 3:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","ok":true}`))
		default:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`<h2>KoboldCpp is not available.</h2>`))
		}
	}))
	service.backendRetryAttempts = 2
	service.backendRetryDelay = 0

	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	firstRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(firstRecorder, firstRequest)

	secondRecorder := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	secondRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(secondRecorder, secondRequest)

	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected first status %d body %s", firstRecorder.Code, firstRecorder.Body.String())
	}
	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected second status %d body %s", secondRecorder.Code, secondRecorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
	if backend.restarts.Load() != 0 {
		t.Fatalf("expected no restarts, got %d", backend.restarts.Load())
	}
}

func TestRetryableStatusLogsBackendBody(t *testing.T) {
	var logs bytes.Buffer
	service, _ := newTestServiceWithLogger(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "kobold generation failed", http.StatusBadGateway)
	}), log.New(&logs, "", 0))
	service.backendRetryAttempts = 1
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(logs.String(), "kobold generation failed") {
		t.Fatalf("expected backend body in logs, got %q", logs.String())
	}
	if !strings.Contains(recorder.Body.String(), "kobold generation failed") {
		t.Fatalf("expected backend body in response, got %s", recorder.Body.String())
	}
}

func TestCoreRequestLoadsModelBeforeForward(t *testing.T) {
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"content":"ready"}}]}`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload before forward, got %d", backend.reloads.Load())
	}
	if backend.lastReload != "a.kcpps" {
		t.Fatalf("unexpected reload config %q", backend.lastReload)
	}
}

func TestModelChangeLoadsNewConfig(t *testing.T) {
	service, backend := newTestServiceWithModels(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"content":"ready"}}]}`))
	}), "a", "b")

	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	firstRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(firstRecorder, firstRequest)

	secondRecorder := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"b","messages":[]}`))
	secondRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(secondRecorder, secondRequest)

	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected first status %d body %s", firstRecorder.Code, firstRecorder.Body.String())
	}
	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected second status %d body %s", secondRecorder.Code, secondRecorder.Body.String())
	}
	if backend.reloads.Load() != 2 {
		t.Fatalf("expected reload for each model, got %d", backend.reloads.Load())
	}
	if backend.lastReload != "b.kcpps" {
		t.Fatalf("unexpected reload config %q", backend.lastReload)
	}
}

func TestSameModelReusesLoadedConfig(t *testing.T) {
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"content":"ready"}}]}`))
	}))

	for range 2 {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
		}
	}

	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload for repeated model, got %d", backend.reloads.Load())
	}
}

func TestStreamingResponseRewritesModel(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"model\":\"koboldcpp/backend\",\"choices\":[]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[],"stream":true}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("unexpected content type %q", recorder.Header().Get("Content-Type"))
	}
	if !strings.Contains(recorder.Body.String(), `"model":"a"`) {
		t.Fatalf("stream model was not rewritten: %q", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "koboldcpp/backend") {
		t.Fatalf("unexpected body %q", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "data: [DONE]") {
		t.Fatalf("done event missing: %q", recorder.Body.String())
	}
}

func TestStreamingResponseEscapesRewrittenModel(t *testing.T) {
	recorder := httptest.NewRecorder()
	response := testHTTPResponse(http.StatusOK, "text/event-stream", "data: {\"model\":\"backend\",\"choices\":[{\"delta\":{\"content\":\"<script>alert(1)</script>\"}}]}\n\n")
	if err := writeProxyResponse(recorder, response, `bad<script>`, true); err != nil {
		t.Fatal(err)
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Fatalf("stream reflected raw model id: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `bad\u003cscript\u003e`) {
		t.Fatalf("stream did not escape model id: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `\u003cscript\u003ealert(1)\u003c/script\u003e`) {
		t.Fatalf("stream did not escape reflected content: %s", recorder.Body.String())
	}
}

func TestStreamingResponseDropsInvalidDataLine(t *testing.T) {
	recorder := httptest.NewRecorder()
	response := testHTTPResponse(http.StatusOK, "text/event-stream", "data: <script>alert(1)</script>\n\n")
	if err := writeProxyResponse(recorder, response, "a", true); err != nil {
		t.Fatal(err)
	}

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Fatalf("invalid event data was reflected: %s", recorder.Body.String())
	}
}

func TestModelProxyResponseHandlesMissingBackendResponse(t *testing.T) {
	tests := []struct {
		name     string
		response *http.Response
	}{
		{name: "nil response"},
		{
			name: "nil body",
			response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			if err := writeModelProxyResponse(recorder, testCase.response, "a", true); err != nil {
				t.Fatal(err)
			}

			if recorder.Code != http.StatusBadGateway {
				t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "backend returned no response") {
				t.Fatalf("missing backend error body: %s", recorder.Body.String())
			}
		})
	}
}

func TestModelRequestRejectsUnsupportedBackendContentType(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<script>alert(1)</script>`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[{"role":"user","content":"<script>alert(1)</script>"}]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Fatalf("unsupported backend response reflected body: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "invalid json") {
		t.Fatalf("missing safe backend error: %s", recorder.Body.String())
	}
}

func TestModelRequestEscapesValidJSONBackendContent(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"<script>alert(1)</script>"}}]}`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[{"role":"user","content":"<script>alert(1)</script>"}]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "<script>") {
		t.Fatalf("json response reflected raw script: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `\u003cscript\u003ealert(1)\u003c/script\u003e`) {
		t.Fatalf("json response did not escape reflected content: %s", recorder.Body.String())
	}
}

func TestEmbeddingsPassThroughWithModelValidation(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"a","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
}

func TestEmbeddingRequestCanUseImageModelAlias(t *testing.T) {
	var sawTextBody bool
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"imageembed"}]}`))
				return
			}
			if r.URL.Path != "/v1/embeddings" {
				t.Fatalf("unexpected text path %s", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			sawTextBody = strings.Contains(string(body), `"model":"imageembed"`) && !strings.Contains(string(body), "perfectdeliberate")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"imageembed","data":[]}`))
		}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatalf("unexpected image path %s", r.URL.Path)
		}),
		map[string]string{
			"imageembed": `{"nomodel":true,"sdmodel":"C:\\models\\perfectdeliberate_v90.safetensors","embeddingsmodel":"C:\\models\\embed.gguf"}`,
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"imageembed-perfectdeliberate_v90","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawTextBody {
		t.Fatalf("backend did not receive the text model alias")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"imageembed-perfectdeliberate_v90"`) {
		t.Fatalf("embedding response model was not rewritten: %s", recorder.Body.String())
	}
	if textBackend.reloads.Load() != 1 || imageBackend.reloads.Load() != 0 {
		t.Fatalf("unexpected reload counts text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}

func TestImageModelListUsesImageEndpoints(t *testing.T) {
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ready"}}]}`))
	}), map[string]string{
		"llm":   `{}`,
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
		"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\vision.safetensors"}`,
	})

	llmRecorder := httptest.NewRecorder()
	llmRequest := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	service.ServeHTTP(llmRecorder, llmRequest)

	if llmRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected llm status %d", llmRecorder.Code)
	}
	if strings.Contains(llmRecorder.Body.String(), `"id":"image"`) {
		t.Fatalf("image-only config should not be in /v1/models: %s", llmRecorder.Body.String())
	}
	if !strings.Contains(llmRecorder.Body.String(), `"id":"combo"`) || !strings.Contains(llmRecorder.Body.String(), `"id":"llm"`) {
		t.Fatalf("llm list missing expected models: %s", llmRecorder.Body.String())
	}

	imageRecorder := httptest.NewRecorder()
	imageRequest := httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil)
	service.ServeHTTP(imageRecorder, imageRequest)

	if imageRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected image status %d", imageRecorder.Code)
	}
	if !strings.Contains(imageRecorder.Body.String(), `"model_name":"image-dream"`) {
		t.Fatalf("image list missing image-only model: %s", imageRecorder.Body.String())
	}
	if strings.Contains(imageRecorder.Body.String(), "combo-vision") {
		t.Fatalf("inactive combined image should not be listed: %s", imageRecorder.Body.String())
	}

	chatRecorder := httptest.NewRecorder()
	chatRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"combo","messages":[]}`))
	chatRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(chatRecorder, chatRequest)
	if chatRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected chat status %d body %s", chatRecorder.Code, chatRecorder.Body.String())
	}

	activeRecorder := httptest.NewRecorder()
	activeRequest := httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil)
	service.ServeHTTP(activeRecorder, activeRequest)

	if !strings.Contains(activeRecorder.Body.String(), `"model_name":"combo-vision"`) {
		t.Fatalf("active combined image should be listed: %s", activeRecorder.Body.String())
	}
}

func TestImageRequestLoadsImageOnlyConfig(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","data":[]}`))
	}), map[string]string{
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-dream","prompt":"cat"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.lastReload != "image.kcpps" {
		t.Fatalf("unexpected reload config %q", backend.lastReload)
	}
	if !strings.Contains(recorder.Body.String(), `"model":"image-dream"`) {
		t.Fatalf("image response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestColdImageRequestWaitsForImageModelsEndpoint(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "image.kcpps"), []byte(`{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var probes atomic.Int32
	var generations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
			w.Header().Set("Content-Type", "application/json")
			if probes.Add(1) < 3 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"model_name":"dream"}]`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/images/generations" {
			if probes.Load() < 3 {
				t.Fatalf("image request forwarded before image models endpoint was ready")
			}
			generations.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","data":[]}`))
			return
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend: backend,
		Catalog: catalog.New(dir),
		Logger:  log.New(io.Discard, "", 0),
	})
	service.backendRetryAttempts = 4
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-dream","prompt":"cat"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if probes.Load() != 3 {
		t.Fatalf("expected three image readiness probes, got %d", probes.Load())
	}
	if generations.Load() != 1 {
		t.Fatalf("expected one image generation request, got %d", generations.Load())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload, got %d", backend.reloads.Load())
	}
}

func TestInactiveCombinedImageModelIsRejected(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\vision.safetensors"}`,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"combo-vision","prompt":"cat"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("expected no reload, got %d", backend.reloads.Load())
	}
}

func TestActiveCombinedImageModelDoesNotReload(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/chat/completions" {
			_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ready"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"backend","data":[]}`))
	}), map[string]string{
		"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\vision.safetensors"}`,
	})

	chatRecorder := httptest.NewRecorder()
	chatRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"combo","messages":[]}`))
	chatRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(chatRecorder, chatRequest)

	imageRecorder := httptest.NewRecorder()
	imageRequest := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"combo-vision","prompt":"cat"}`))
	imageRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(imageRecorder, imageRequest)

	if chatRecorder.Code != http.StatusOK || imageRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected statuses chat=%d image=%d imageBody=%s", chatRecorder.Code, imageRecorder.Code, imageRecorder.Body.String())
	}
	if backend.reloads.Load() != 1 {
		t.Fatalf("expected one reload for combined config, got %d", backend.reloads.Load())
	}
}

func TestImageConfigSwitchWaitsForStreamingLLMRequest(t *testing.T) {
	chatStarted := make(chan struct{})
	releaseChat := make(chan struct{})
	imageReloaded := make(chan struct{})
	var imageReloadOnce sync.Once

	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"model\":\"backend\",\"choices\":[]}\n\n"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(chatStarted)
			<-releaseChat
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"backend","data":[]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}), map[string]string{
		"llm":   `{}`,
		"image": `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`,
	})
	backend.onReload = func(filename string) {
		if filename == "image.kcpps" {
			imageReloadOnce.Do(func() {
				close(imageReloaded)
			})
		}
	}

	chatDone := make(chan struct{})
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"llm","messages":[],"stream":true}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		close(chatDone)
	}()

	select {
	case <-chatStarted:
	case <-time.After(time.Second):
		t.Fatal("chat did not start")
	}

	imageDone := make(chan struct{})
	go func() {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-dream","prompt":"cat"}`))
		request.Header.Set("Content-Type", "application/json")
		service.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Errorf("unexpected image status %d body %s", recorder.Code, recorder.Body.String())
		}
		close(imageDone)
	}()

	select {
	case <-imageReloaded:
		t.Fatal("image config reloaded while llm stream was active")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseChat)

	select {
	case <-imageDone:
	case <-time.After(time.Second):
		t.Fatal("image request did not finish")
	}
	select {
	case <-chatDone:
	case <-time.After(time.Second):
		t.Fatal("chat request did not finish")
	}
	if backend.reloads.Load() != 2 {
		t.Fatalf("expected llm and image reloads, got %d", backend.reloads.Load())
	}
}

func TestClusterModelsEndpointHidesNodeIdentityAndIndexesConflicts(t *testing.T) {
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterModel("same", "master", "master-hash", "config-hash", cluster.SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: "http://slave-a",
		Models:  []cluster.Model{testClusterModel("same", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"id":"same"`) || !strings.Contains(recorder.Body.String(), `"id":"same-2"`) {
		t.Fatalf("missing public models: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "slave-a") {
		t.Fatalf("node identity leaked: %s", recorder.Body.String())
	}
}

func TestClusterRemoteRequestRewritesModelBothWays(t *testing.T) {
	var sawAuthorization bool
	var sawLocalModel bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected remote path %s", r.URL.Path)
		}
		sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawLocalModel = strings.Contains(string(body), `"model":"same"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterModel("same", "master", "master-hash", "config-hash", cluster.SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: remote.URL,
		Models:  []cluster.Model{testClusterModel("same", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"same-2","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawAuthorization {
		t.Fatalf("cluster authorization was not forwarded")
	}
	if !sawLocalModel {
		t.Fatalf("remote did not receive local model id")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"same-2"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestClusterImageModelListUsesPublicImageIDs(t *testing.T) {
	registry := newConflictingImageRegistry(t, "http://slave-a")
	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"model_name":"same-dream"`) || !strings.Contains(recorder.Body.String(), `"model_name":"same-2-dream"`) {
		t.Fatalf("missing public image models: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "slave-a") {
		t.Fatalf("node identity leaked: %s", recorder.Body.String())
	}
}

func TestClusterRemoteImageRequestRewritesModelBothWays(t *testing.T) {
	var sawAuthorization bool
	var sawLocalImageModel bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected remote path %s", r.URL.Path)
		}
		sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawLocalImageModel = strings.Contains(string(body), `"model":"same-dream"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"same-dream","data":[]}`))
	}))
	defer remote.Close()

	registry := newConflictingImageRegistry(t, remote.URL)
	service, backend := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"same-2-dream","prompt":"cat"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawAuthorization {
		t.Fatalf("cluster authorization was not forwarded")
	}
	if !sawLocalImageModel {
		t.Fatalf("remote did not receive local image model id")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"same-2-dream"`) {
		t.Fatalf("image response model was not rewritten: %s", recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("local backend should not reload for remote image request, got %d", backend.reloads.Load())
	}
}

func TestClusterRemoteImageRequestRewritesSelectors(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		body           string
		contentType    string
		headerModel    string
		queryKey       string
		expectedBody   string
		expectedQuery  string
		expectedHeader string
	}{
		{
			name:         "sd checkpoint body",
			path:         "/sdapi/v1/txt2img",
			body:         `{"sd_model_checkpoint":"same-2-dream","prompt":"cat"}`,
			contentType:  "application/json",
			expectedBody: `"sd_model_checkpoint":"same-dream"`,
		},
		{
			name:         "override checkpoint body",
			path:         "/sdapi/v1/txt2img",
			body:         `{"override_settings":{"sd_model_checkpoint":"same-2-dream"},"prompt":"cat"}`,
			contentType:  "application/json",
			expectedBody: `"sd_model_checkpoint":"same-dream"`,
		},
		{
			name:          "model query",
			path:          "/v1/images/generations?model=same-2-dream",
			body:          `{"prompt":"cat"}`,
			contentType:   "application/json",
			queryKey:      "model",
			expectedQuery: "same-dream",
		},
		{
			name:          "sd checkpoint query",
			path:          "/sdapi/v1/txt2img?sd_model_checkpoint=same-2-dream",
			body:          `{"prompt":"cat"}`,
			contentType:   "application/json",
			queryKey:      "sd_model_checkpoint",
			expectedQuery: "same-dream",
		},
		{
			name:           "model header",
			path:           "/sdapi/v1/txt2img",
			body:           `{"prompt":"cat"}`,
			contentType:    "application/json",
			headerModel:    "same-2-dream",
			expectedHeader: "same-dream",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if testCase.expectedBody != "" && !strings.Contains(string(body), testCase.expectedBody) {
					t.Fatalf("remote body missing rewritten selector: %s", string(body))
				}
				if testCase.queryKey != "" && r.URL.Query().Get(testCase.queryKey) != testCase.expectedQuery {
					t.Fatalf("remote query was not rewritten: %s", r.URL.RawQuery)
				}
				if testCase.expectedHeader != "" && r.Header.Get("X-Tensors-Model") != testCase.expectedHeader {
					t.Fatalf("remote header was not rewritten: %s", r.Header.Get("X-Tensors-Model"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"model":"same-dream","data":[]}`))
			}))
			defer remote.Close()

			registry := newConflictingImageRegistry(t, remote.URL)
			service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, testCase.path, strings.NewReader(testCase.body))
			if testCase.contentType != "" {
				request.Header.Set("Content-Type", testCase.contentType)
			}
			if testCase.headerModel != "" {
				request.Header.Set("X-Tensors-Model", testCase.headerModel)
			}
			service.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), `"model":"same-2-dream"`) {
				t.Fatalf("image response model was not rewritten: %s", recorder.Body.String())
			}
		})
	}
}

func TestClusterInactiveLocalCombinedImageModelIsRejected(t *testing.T) {
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), map[string]string{
		"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\vision.safetensors"}`,
	})
	models, err := service.catalog.List()
	if err != nil {
		t.Fatal(err)
	}
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal(cluster.LocalModels(models, "master", "http://master", cluster.SourceMaster)); err != nil {
		t.Fatal(err)
	}
	service.registry = registry

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"combo-vision","prompt":"cat"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backend.reloads.Load() != 0 {
		t.Fatalf("expected no reload, got %d", backend.reloads.Load())
	}
}

func TestLlamaSDCPPRoutesTextAndImagesToSeparateBackends(t *testing.T) {
	var textPosts atomic.Int32
	var imagePosts atomic.Int32
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
				return
			}
			if r.URL.Path != "/v1/chat/completions" {
				t.Fatalf("unexpected text path %s", r.URL.Path)
			}
			textPosts.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"backend","choices":[{"message":{"content":"ok"}}]}`))
		}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
				return
			}
			if r.URL.Path != "/v1/images/generations" {
				t.Fatalf("unexpected image path %s", r.URL.Path)
			}
			imagePosts.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"model":"backend","data":[]}`))
		}),
		map[string]string{
			"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\dream.safetensors"}`,
		},
	)

	listRecorder := httptest.NewRecorder()
	service.ServeHTTP(listRecorder, httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil))
	if listRecorder.Code != http.StatusOK || !strings.Contains(listRecorder.Body.String(), `"model_name":"combo-dream"`) {
		t.Fatalf("combined image was not listed in split mode: status=%d body=%s", listRecorder.Code, listRecorder.Body.String())
	}

	textRecorder := httptest.NewRecorder()
	textRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"combo","messages":[]}`))
	textRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(textRecorder, textRequest)
	if textRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected text status %d body %s", textRecorder.Code, textRecorder.Body.String())
	}

	imageRecorder := httptest.NewRecorder()
	imageRequest := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"combo-dream","prompt":"cat"}`))
	imageRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(imageRecorder, imageRequest)
	if imageRecorder.Code != http.StatusOK {
		t.Fatalf("unexpected image status %d body %s", imageRecorder.Code, imageRecorder.Body.String())
	}

	if textPosts.Load() != 1 || imagePosts.Load() != 1 {
		t.Fatalf("unexpected forward counts text=%d image=%d", textPosts.Load(), imagePosts.Load())
	}
	if textBackend.reloads.Load() != 1 || imageBackend.reloads.Load() != 1 {
		t.Fatalf("unexpected reload counts text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}

func TestLlamaSDCPPRouterLoadCombinedConfigLoadsBothLanes(t *testing.T) {
	service, textBackend, imageBackend := newSplitTestServiceWithConfigContents(
		t,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"backend"}]}`))
				return
			}
			t.Fatalf("unexpected text path %s", r.URL.Path)
		}),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
				return
			}
			t.Fatalf("unexpected image path %s", r.URL.Path)
		}),
		map[string]string{
			"combo": `{"model_param":"C:\\models\\llm.gguf","sdmodel":"C:\\models\\dream.safetensors"}`,
		},
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/load", strings.NewReader(`{"model":"combo"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if textBackend.reloads.Load() != 1 || imageBackend.reloads.Load() != 1 {
		t.Fatalf("expected both lanes to reload, got text=%d image=%d", textBackend.reloads.Load(), imageBackend.reloads.Load())
	}
}

func TestClusterSplitCombinedImageModelIsVisibleWhenInactive(t *testing.T) {
	model := testClusterModel("combo", "master", "model-hash", "config-hash", cluster.SourceMaster)
	model.HasImage = true
	model.ImageID = "combo-dream"
	model.PublicImageID = "combo-dream"
	model.BackendMode = BackendModeLlamaSDCPP
	model.Capabilities.Image = &catalog.ImageCapabilities{Model: "C:/models/dream.safetensors"}

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{model}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	service.backendMode = BackendModeLlamaSDCPP
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sdapi/v1/sd-models", nil)
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"model_name":"combo-dream"`) {
		t.Fatalf("combined split image model was hidden: %s", recorder.Body.String())
	}
}

func TestClusterRemoteEmbeddingsRequestRewritesModelBothWays(t *testing.T) {
	var sawAuthorization bool
	var sawLocalModel bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected remote path %s", r.URL.Path)
		}
		sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawLocalModel = strings.Contains(string(body), `"model":"embed"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"embed","data":[]}`))
	}))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterEmbeddingModel("embed", "master", "master-hash", "config-hash", cluster.SourceMaster)}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: remote.URL,
		Models:  []cluster.Model{testClusterEmbeddingModel("embed", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave)},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"embed-2","input":"hello"}`))
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawAuthorization {
		t.Fatalf("cluster authorization was not forwarded")
	}
	if !sawLocalModel {
		t.Fatalf("remote did not receive local embedding model id")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"embed-2"`) {
		t.Fatalf("embedding response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestClusterRemoteEmbeddingsRequestCanUsePublicImageID(t *testing.T) {
	var sawAuthorization bool
	var sawLocalModel bool
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected remote path %s", r.URL.Path)
		}
		sawAuthorization = r.Header.Get("Authorization") == "Bearer secret"
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		sawLocalModel = strings.Contains(string(body), `"model":"imageembed"`) && !strings.Contains(string(body), "perfectdeliberate")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"imageembed","data":[]}`))
	}))
	defer remote.Close()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: remote.URL,
		Models:  []cluster.Model{testClusterImageEmbeddingModel("imageembed", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave, "perfectdeliberate_v90")},
	}); err != nil {
		t.Fatal(err)
	}

	service, _ := newTestServiceWithRegistry(t, registry, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), "secret")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(`{"model":"imageembed-perfectdeliberate_v90","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawAuthorization {
		t.Fatalf("cluster authorization was not forwarded")
	}
	if !sawLocalModel {
		t.Fatalf("remote did not receive local embedding model id")
	}
	if !strings.Contains(recorder.Body.String(), `"model":"imageembed-perfectdeliberate_v90"`) {
		t.Fatalf("embedding response model was not rewritten: %s", recorder.Body.String())
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

func newTestService(t *testing.T, backendHandler http.Handler) (*Service, *fakeBackend) {
	return newTestServiceWithLogger(t, backendHandler, log.New(io.Discard, "", 0))
}

func writeProxyTestConfig(t *testing.T, dir string, id string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id+".kcpps"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newReadyFakeBackend(t *testing.T, onReload func(string)) *fakeBackend {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
		case "/sdapi/v1/sd-models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
		default:
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeBackend{url: backendURL, healthy: true, onReload: onReload}
}

func eventsSnapshot(mu *sync.Mutex, events *[]string) []string {
	mu.Lock()
	defer mu.Unlock()
	return append([]string{}, (*events)...)
}

func eventBefore(events []string, first string, second string) bool {
	firstIndex := -1
	secondIndex := -1
	for index, event := range events {
		if event == first && firstIndex == -1 {
			firstIndex = index
		}
		if event == second && secondIndex == -1 {
			secondIndex = index
		}
	}
	return firstIndex >= 0 && secondIndex >= 0 && firstIndex < secondIndex
}

func newTestServiceWithModels(t *testing.T, backendHandler http.Handler, modelIDs ...string) (*Service, *fakeBackend) {
	return newTestServiceWithModelsAndLogger(t, backendHandler, log.New(io.Discard, "", 0), modelIDs...)
}

func newTestServiceWithLogger(t *testing.T, backendHandler http.Handler, logger *log.Logger) (*Service, *fakeBackend) {
	return newTestServiceWithModelsAndLogger(t, backendHandler, logger, "a")
}

func newTestServiceWithModelsAndLogger(t *testing.T, backendHandler http.Handler, logger *log.Logger, modelIDs ...string) (*Service, *fakeBackend) {
	t.Helper()

	return newTestServiceWithBackendSetup(t, backendHandler, logger, true, modelIDs...)
}

func newTestServiceWithRegistry(t *testing.T, registry *cluster.Registry, backendHandler http.Handler, token string) (*Service, *fakeBackend) {
	t.Helper()

	server := httptest.NewServer(backendHandler)
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend:      backend,
		Catalog:      catalog.New(t.TempDir()),
		Registry:     registry,
		ClusterToken: token,
		Logger:       log.New(io.Discard, "", 0),
	})
	return service, backend
}

func newTestServiceWithRawBackend(t *testing.T, backendHandler http.Handler, modelIDs ...string) (*Service, *fakeBackend) {
	t.Helper()

	return newTestServiceWithBackendSetup(t, backendHandler, log.New(io.Discard, "", 0), false, modelIDs...)
}

func newTestServiceWithConfigContents(t *testing.T, backendHandler http.Handler, configs map[string]string) (*Service, *fakeBackend) {
	t.Helper()

	dir := t.TempDir()
	for modelID, content := range configs {
		if err := os.WriteFile(filepath.Join(dir, modelID+".kcpps"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/sdapi/v1/sd-models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"model_name":"ready"}]`))
			return
		}
		backendHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend: backend,
		Catalog: catalog.New(dir),
		Logger:  log.New(io.Discard, "", 0),
	})
	return service, backend
}

func newSplitTestServiceWithConfigContents(t *testing.T, textHandler http.Handler, imageHandler http.Handler, configs map[string]string) (*Service, *fakeBackend, *fakeBackend) {
	t.Helper()

	dir := t.TempDir()
	for modelID, content := range configs {
		if err := os.WriteFile(filepath.Join(dir, modelID+".kcpps"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	textServer := httptest.NewServer(textHandler)
	t.Cleanup(textServer.Close)
	imageServer := httptest.NewServer(imageHandler)
	t.Cleanup(imageServer.Close)
	textURL, err := url.Parse(textServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageURL, err := url.Parse(imageServer.URL)
	if err != nil {
		t.Fatal(err)
	}

	textBackend := &fakeBackend{url: textURL, healthy: true}
	imageBackend := &fakeBackend{url: imageURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend:      textBackend,
		TextBackend:  textBackend,
		ImageBackend: imageBackend,
		BackendMode:  BackendModeLlamaSDCPP,
		Catalog:      catalog.New(dir),
		Logger:       log.New(io.Discard, "", 0),
	})
	return service, textBackend, imageBackend
}

func testClusterModel(id string, nodeID string, modelHash string, configHash string, source string) cluster.Model {
	return cluster.Model{
		PublicID:   id,
		LocalID:    id,
		Filename:   id + ".kcpps",
		Created:    1,
		HasLLM:     true,
		ModelHash:  modelHash,
		ConfigHash: configHash,
		Source:     source,
		NodeID:     nodeID,
		NodeURL:    "http://" + nodeID,
		Available:  true,
	}
}

func testClusterImageModel(id string, nodeID string, modelHash string, configHash string, source string, imageName string) cluster.Model {
	model := testClusterModel(id, nodeID, modelHash, configHash, source)
	model.HasLLM = false
	model.HasImage = true
	model.ImageID = id + "-" + imageName
	model.PublicImageID = model.ImageID
	model.Capabilities.Image = &catalog.ImageCapabilities{
		Model: "C:/models/" + imageName + ".safetensors",
	}
	return model
}

func testClusterEmbeddingModel(id string, nodeID string, modelHash string, configHash string, source string) cluster.Model {
	model := testClusterModel(id, nodeID, modelHash, configHash, source)
	model.HasLLM = false
	model.HasEmbeddings = true
	model.Capabilities.Embeddings = &catalog.EmbeddingCapability{
		Model: "C:/models/" + id + ".gguf",
	}
	return model
}

func testClusterImageEmbeddingModel(id string, nodeID string, modelHash string, configHash string, source string, imageName string) cluster.Model {
	model := testClusterEmbeddingModel(id, nodeID, modelHash, configHash, source)
	model.HasImage = true
	model.ImageID = id + "-" + imageName
	model.PublicImageID = model.ImageID
	model.BackendMode = BackendModeLlamaSDCPP
	model.Capabilities.Image = &catalog.ImageCapabilities{
		Model: "C:/models/" + imageName + ".safetensors",
	}
	return model
}

func newConflictingImageRegistry(t *testing.T, slaveURL string) *cluster.Registry {
	t.Helper()

	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "http://master")
	if err := registry.UpdateLocal([]cluster.Model{testClusterImageModel("same", "master", "master-hash", "config-hash", cluster.SourceMaster, "dream")}); err != nil {
		t.Fatal(err)
	}
	if err := registry.UpdateNode(cluster.Snapshot{
		NodeID:  "slave-a",
		NodeURL: slaveURL,
		Models:  []cluster.Model{testClusterImageModel("same", "slave-a", "slave-hash", "config-hash", cluster.SourceSlave, "dream")},
	}); err != nil {
		t.Fatal(err)
	}
	return registry
}

func newTestServiceWithBackendSetup(t *testing.T, backendHandler http.Handler, logger *log.Logger, addModelsEndpoint bool, modelIDs ...string) (*Service, *fakeBackend) {
	t.Helper()

	dir := t.TempDir()
	for _, modelID := range modelIDs {
		if err := os.WriteFile(filepath.Join(dir, modelID+".kcpps"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if addModelsEndpoint && r.Method == http.MethodGet && r.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
			return
		}
		backendHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	backendURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		Backend: backend,
		Catalog: catalog.New(dir),
		Logger:  logger,
	})
	return service, backend
}
