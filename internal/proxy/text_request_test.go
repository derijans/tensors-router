package proxy

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestOllamaStatusEndpointsAreSynthesizedWithoutBackendLeakage(t *testing.T) {
	endpoints := map[string]string{"/api/tags": `"models"`, "/api/ps": `"models"`, "/api/version": `"version"`}
	for endpoint, responseField := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			var sawRequest bool
			service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sawRequest = true
			}), map[string]string{})

			recorder := httptest.NewRecorder()
			service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, endpoint, nil))

			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), responseField) {
				t.Fatalf("unexpected response status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if sawRequest {
				t.Fatal("Ollama status endpoint leaked to backend")
			}
			if backend.reloads.Load() != 0 {
				t.Fatalf("status endpoint reloaded config %d times", backend.reloads.Load())
			}
		})
	}
}

func TestRequiredTextEndpointRejectsMissingModel(t *testing.T) {
	var backendCalled bool
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
	}), map[string]string{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backendCalled {
		t.Fatalf("backend should not receive missing-model request")
	}
}

func TestAPIAdminPathsAreNotPubliclyProxied(t *testing.T) {
	var backendCalled bool
	service, _ := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendCalled = true
	}), map[string]string{})

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/admin/reload_config", strings.NewReader(`{}`)))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if backendCalled {
		t.Fatalf("admin path was proxied")
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

func TestModelAwareTextEndpointsRouteSelectedConfig(t *testing.T) {
	endpoints := []string{
		"/v1/responses",
		"/v1/responses/input_tokens",
		"/v1/messages",
		"/v1/messages/count_tokens",
		"/v1/rerank",
		"/v1/reranking",
		"/api/v1/generate",
		"/api/extra/generate/stream",
		"/api/extra/embeddings",
		"/api/extra/tokencount",
		"/api/generate",
		"/api/chat",
	}
	for _, endpoint := range endpoints {
		t.Run(strings.Trim(endpoint, "/"), func(t *testing.T) {
			assertTextEndpointRoutesSelectedConfig(t, endpoint)
		})
	}
}

func assertTextEndpointRoutesSelectedConfig(t *testing.T, endpoint string) {
	var sawRequest bool
	service, backend := newTestServiceWithConfigContents(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == koboldPerfPath {
			_, _ = w.Write([]byte(`{"total_gens":0}`))
			return
		}
		sawRequest = forwardedBodyNamesModel(t, r, endpoint, "text")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"backend","ok":true}`))
	}), map[string]string{
		"text": `{"model_param":"C:\models\text.gguf"}`,
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"model":"text","prompt":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if !sawRequest {
		t.Fatalf("backend did not receive rewritten local model")
	}
	if backend.lastReload != "text.kcpps" {
		t.Fatalf("unexpected reload config %q", backend.lastReload)
	}
	if !strings.Contains(recorder.Body.String(), `"model":"text"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
}

func forwardedBodyNamesModel(t *testing.T, r *http.Request, expectedPath string, model string) bool {
	if r.URL.Path != expectedPath {
		t.Fatalf("unexpected path %s", r.URL.Path)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(body), `"model":"`+model+`"`)
}
