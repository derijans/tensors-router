package proxy

import (
	"bytes"
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

type fakeBackend struct {
	url        *url.URL
	reloads    atomic.Int32
	restarts   atomic.Int32
	healthy    bool
	lastReload string
}

func (backend *fakeBackend) URL() *url.URL {
	copyValue := *backend.url
	return &copyValue
}

func (backend *fakeBackend) ReloadConfig(ctx context.Context, filename string) error {
	backend.lastReload = filename
	backend.reloads.Add(1)
	return nil
}

func (backend *fakeBackend) Restart(ctx context.Context) error {
	backend.restarts.Add(1)
	return nil
}

func (backend *fakeBackend) Healthy(ctx context.Context) bool {
	return backend.healthy
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

func newTestService(t *testing.T, backendHandler http.Handler) (*Service, *fakeBackend) {
	return newTestServiceWithLogger(t, backendHandler, log.New(io.Discard, "", 0))
}

func newTestServiceWithModels(t *testing.T, backendHandler http.Handler, modelIDs ...string) (*Service, *fakeBackend) {
	return newTestServiceWithModelsAndLogger(t, backendHandler, log.New(io.Discard, "", 0), modelIDs...)
}

func newTestServiceWithLogger(t *testing.T, backendHandler http.Handler, logger *log.Logger) (*Service, *fakeBackend) {
	return newTestServiceWithModelsAndLogger(t, backendHandler, logger, "a")
}

func newTestServiceWithModelsAndLogger(t *testing.T, backendHandler http.Handler, logger *log.Logger, modelIDs ...string) (*Service, *fakeBackend) {
	t.Helper()

	dir := t.TempDir()
	for _, modelID := range modelIDs {
		if err := os.WriteFile(filepath.Join(dir, modelID+".kcpps"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	server := httptest.NewServer(backendHandler)
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
