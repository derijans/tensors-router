package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestVLLMAdminAllowlistAndFeatureGates(t *testing.T) {
	var backendPaths []string
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendPaths = append(backendPaths, r.URL.Path)
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatalf("credentials reached vLLM: %#v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer backendServer.Close()
	backendURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{url: backendURL, healthy: true}
	service := NewService(ServiceConfig{
		BackendMode: BackendModeVLLM,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeVLLM: {TextBackend: backend, EmbeddingsBackend: backend, TranscriptionBackend: backend},
		},
	})
	service.textRuntime.state.filename = "generation.kcpps"

	health := httptest.NewRequest(http.MethodGet, "/router/v1/vllm/health", nil)
	health.Header.Set("Authorization", "Bearer admin-secret")
	health.Header.Set("Cookie", "session=secret")
	healthRecorder := httptest.NewRecorder()
	service.ServeHTTP(healthRecorder, health)
	if healthRecorder.Code != http.StatusOK || len(backendPaths) != 1 || backendPaths[0] != "/health" {
		t.Fatalf("health was not safely proxied: status=%d paths=%#v body=%s", healthRecorder.Code, backendPaths, healthRecorder.Body.String())
	}

	for _, path := range []string{"/router/v1/vllm/lora/load", "/router/v1/vllm/eep/scale"} {
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`)))
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("disabled feature %s returned %d", path, recorder.Code)
		}
	}
	for _, path := range []string{"/router/v1/vllm/start-profile", "/router/v1/vllm/rpc", "/router/v1/vllm/sleep"} {
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("unsafe operation %s returned %d", path, recorder.Code)
		}
	}
	if len(backendPaths) != 1 {
		t.Fatalf("blocked operations reached vLLM: %#v", backendPaths)
	}
}

func TestVLLMAdminUsesSelectedLoadedRuntime(t *testing.T) {
	generationURL, _ := url.Parse("http://generation.invalid")
	poolingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tokenizer_info" {
			t.Fatalf("unexpected backend path %q", r.URL.Path)
		}
		_, _ = w.Write([]byte("pooling"))
	}))
	defer poolingServer.Close()
	poolingURL, _ := url.Parse(poolingServer.URL)
	service := NewService(ServiceConfig{
		BackendMode: BackendModeVLLM,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeVLLM: {
				TextBackend:       &fakeBackend{url: generationURL},
				EmbeddingsBackend: &fakeBackend{url: poolingURL, healthy: true},
			},
		},
	})
	service.embeddingsRuntime.state.filename = "pooling.kcpps"

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/vllm/tokenizer-info?runtime=pooling", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "pooling" {
		t.Fatalf("pooling operation failed: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestVLLMAdminRejectsOversizedRequestBody(t *testing.T) {
	backendURL, _ := url.Parse("http://generation.invalid")
	service := NewService(ServiceConfig{
		BackendMode: BackendModeVLLM,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeVLLM: {TextBackend: &fakeBackend{url: backendURL, healthy: true}},
		},
		VLLMDynamicLoRAEnabled: true,
	})
	service.textRuntime.state.filename = "generation.kcpps"
	service.transportLimits.MaxRequestBytes = 4
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/router/v1/vllm/lora/load", strings.NewReader("12345")))
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized admin request returned %d: %s", recorder.Code, recorder.Body.String())
	}
}
