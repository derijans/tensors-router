package proxy

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

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
