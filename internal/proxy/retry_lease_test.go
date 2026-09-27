package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRetryHoldsTheRuntimeInsteadOfReloading(t *testing.T) {
	var requests atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if requests.Add(1) == 2 {
			// Retryable, but not a "backend warming" shape and not a health failure:
			// exactly the case that used to trigger release-and-force-reload.
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"transient backend failure"}`))
			return
		}
		_, _ = w.Write([]byte(`{"model":"koboldcpp/backend","choices":[{"message":{"content":"recovered"}}]}`))
	}))
	service.backendRetryDelay = 0

	// Warm the runtime first. The reload-on-retry branch only applies when the config was
	// already loaded; a request that loads it fresh never reaches it.
	warm := httptest.NewRecorder()
	warmRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	warmRequest.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(warm, warmRequest)
	if warm.Code != http.StatusOK {
		t.Fatalf("warm-up status = %d body %s", warm.Code, warm.Body.String())
	}
	reloadsAfterWarm := backend.reloads.Load()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"a","messages":[]}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "recovered") {
		t.Fatalf("retry did not return the recovered response: %s", recorder.Body.String())
	}
	if requests.Load() != 3 {
		t.Fatalf("expected warm-up plus one retried request (3 backend calls), got %d", requests.Load())
	}
	if reloads := backend.reloads.Load() - reloadsAfterWarm; reloads != 0 {
		t.Fatalf("retry reloaded the runtime %d times; it must retry against the config it already holds", reloads)
	}
	if unloads := backend.unloads.Load(); unloads != 0 {
		t.Fatalf("retry unloaded the runtime %d times", unloads)
	}
}
