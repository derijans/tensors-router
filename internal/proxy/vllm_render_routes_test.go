package proxy

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"tensors-router/internal/catalog"
)

var vllmRenderPaths = []string{"/v1/messages/render", "/v1/responses/render", "/cohere/v2/chat/render"}

func TestVLLMRenderRoutesReachGenerationRuntime(t *testing.T) {
	for _, path := range vllmRenderPaths {
		t.Run(path, func(t *testing.T) {
			var renderRequests atomic.Int32
			service := newVLLMGenerationService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == path {
					renderRequests.Add(1)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"rendered":true}`)
			}))
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"model","input":"hello"}`))
			request.Header.Set("Content-Type", "application/json")
			service.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"rendered":true`) {
				t.Fatalf("unexpected render response status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if renderRequests.Load() != 1 {
				t.Fatalf("runtime received %d render requests", renderRequests.Load())
			}
		})
	}
}

func TestVLLMRenderRoutesRequireModel(t *testing.T) {
	for _, path := range vllmRenderPaths {
		if !isTextPath(path) || !textPathRequiresModel(path) {
			t.Errorf("render path %s is not a model-selected text path", path)
		}
	}
}

func TestVLLMRenderRoutesAllowOnlyPost(t *testing.T) {
	for _, path := range vllmRenderPaths {
		if !vllmInferenceAllowed(http.MethodPost, path) {
			t.Errorf("POST %s blocked by allowlist", path)
		}
		if vllmInferenceAllowed(http.MethodGet, path) {
			t.Errorf("GET %s allowed by allowlist", path)
		}
	}
}

func TestVLLMResponseRenderIsNotAResponseOperation(t *testing.T) {
	if _, _, operation := vllmResponseOperation("/v1/responses/render"); operation {
		t.Fatal("render endpoint parsed as a stored response ID")
	}
	operations := []struct{ path, id, action string }{
		{"/v1/responses/resp-1", "resp-1", ""},
		{"/v1/responses/resp-1/cancel", "resp-1", "cancel"},
	}
	for _, operation := range operations {
		id, action, found := vllmResponseOperation(operation.path)
		if !found || id != operation.id || action != operation.action {
			t.Errorf("path %s parsed as id=%q action=%q found=%t", operation.path, id, action, found)
		}
	}
}

func TestVLLMStoredResponseOperationsStillUseResponseOwnership(t *testing.T) {
	service := NewService(ServiceConfig{Logger: log.New(io.Discard, "", 0)})
	defer service.Close(t.Context())
	requests := []struct{ method, path string }{
		{http.MethodGet, "/v1/responses/resp-unknown"},
		{http.MethodDelete, "/v1/responses/resp-unknown"},
		{http.MethodPost, "/v1/responses/resp-unknown/cancel"},
	}
	for _, request := range requests {
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, nil))
		if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"type":"response_not_found"`) {
			t.Errorf("%s %s bypassed response ownership status=%d body=%s", request.method, request.path, recorder.Code, recorder.Body.String())
		}
	}
}

func newVLLMGenerationService(t *testing.T, runtime http.Handler) *Service {
	t.Helper()
	backendServer := httptest.NewServer(runtime)
	t.Cleanup(backendServer.Close)
	backendURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{url: backendURL, healthy: true}
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "model.kcpps"), []byte(`{"backend_mode":"vllm","vllm":{"task":"generate"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(ServiceConfig{
		BackendMode: BackendModeVLLM,
		BackendFamilies: map[string]BackendFamilyConfig{
			BackendModeVLLM: {TextBackend: backend, EmbeddingsBackend: backend, TranscriptionBackend: backend},
		},
		Catalog:   catalog.New(configDir),
		ConfigDir: configDir,
		Logger:    log.New(io.Discard, "", 0),
	})
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	return service
}
