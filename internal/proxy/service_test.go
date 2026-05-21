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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/catalog"
)

type fakeBackend struct {
	url        *url.URL
	reloads    atomic.Int32
	restarts   atomic.Int32
	healthy    bool
	lastReload string
	onReload   func(string)
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

	return newTestServiceWithBackendSetup(t, backendHandler, logger, true, modelIDs...)
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
		if r.Method == http.MethodGet && (r.URL.Path == "/v1/models" || r.URL.Path == "/sdapi/v1/sd-models") {
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
		Logger:  log.New(io.Discard, "", 0),
	})
	return service, backend
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
