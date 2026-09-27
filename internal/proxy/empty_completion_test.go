package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReasoningOnlyChatCompletionIsNotRetried(t *testing.T) {
	var requests atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"role":"assistant","content":"","reasoning_content":"Let me think about this."},"finish_reason":"stop"}]}`))
	}))
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body %s", recorder.Code, recorder.Body.String())
	}
	if requests.Load() != 1 {
		t.Fatalf("expected one backend request, got %d", requests.Load())
	}
	const initialConfigAcquireReloads = 1
	if backend.reloads.Load() != initialConfigAcquireReloads {
		t.Fatalf("expected only the initial load, got %d reloads", backend.reloads.Load())
	}
	if !strings.Contains(recorder.Body.String(), "Let me think about this.") {
		t.Fatalf("reasoning content was dropped: %s", recorder.Body.String())
	}
}

func TestPersistentlyEmptyCompletionIsReturnedNotFailed(t *testing.T) {
	var requests atomic.Int32
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"role":"assistant","content":""},"finish_reason":"stop"}]}`))
	}))
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"finish_reason":"stop"`) {
		t.Fatalf("empty completion was not passed through: %s", recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"model":"a"`) {
		t.Fatalf("response model was not rewritten: %s", recorder.Body.String())
	}
}

func TestEmptyCompletionUsesSmallInferenceRetryBudget(t *testing.T) {
	var requests atomic.Int32
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"role":"assistant","content":""}}]}`))
	}))
	service.backendRetryAttempts = 300
	service.backendInferenceRetryAttempts = 3
	service.backendRetryDelay = 0

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	const initialAttempt = 1
	if total := requests.Load(); total > initialAttempt+defaultBackendInferenceRetryAttempts {
		t.Fatalf("backend was called %d times, want at most %d", total, initialAttempt+defaultBackendInferenceRetryAttempts)
	}
}
