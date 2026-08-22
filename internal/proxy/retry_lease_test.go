package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// A retry must not hand the runtime back between attempts.
//
// Releasing the lease to force a reload opened a window in which a request wanting a
// different model could take the runtime and switch it; the retry then switched it back.
// With two models in play the pair traded the runtime until the retry budget ran out.
// Telemetry showed the consequence plainly: of the requests that ended in 502, 83% had a
// model load overlapping them, against 1.3% of the ones that succeeded. Long requests were
// the visible victims only because being in flight longer exposed them to someone else's
// switch.
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
	// Any reload beyond the warm-up means the retry released the runtime and reloaded it,
	// which is the window that lets another model steal it.
	if reloads := backend.reloads.Load() - reloadsAfterWarm; reloads != 0 {
		t.Fatalf("retry reloaded the runtime %d times; it must retry against the config it already holds", reloads)
	}
	if unloads := backend.unloads.Load(); unloads != 0 {
		t.Fatalf("retry unloaded the runtime %d times", unloads)
	}
}
