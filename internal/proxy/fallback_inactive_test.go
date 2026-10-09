package proxy

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFallbackWaitsAfterInactiveCoreResponse(t *testing.T) {
	var probes atomic.Int32
	var posts atomic.Int32
	service, backend := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	service.client = &http.Client{Transport: inactiveThenReadyTransport(t, &probes, &posts)}
	service.backendRetryAttempts = 5
	service.backendRetryDelay = 0

	recorder := expectProxyStatus(t, service, modelJSONRequest("/v1/chat/completions", `{"model":"a","messages":[]}`, ""), http.StatusOK, "chat")
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

func inactiveThenReadyTransport(t *testing.T, probes *atomic.Int32, posts *atomic.Int32) roundTripFunc {
	return func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			return testHTTPResponse(http.StatusOK, "application/json", inactiveModelsBody(probes.Add(1))), nil
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			if posts.Add(1) == 1 {
				return testHTTPResponse(http.StatusOK, "application/json", `{"id":"chatcmpl-test","object":"chat.completion","model":"inactive","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[]},"finish_reason":"error","logprobs":null}]}`), nil
			}
			return testHTTPResponse(http.StatusOK, "application/json", `{"model":"koboldcpp/backend","choices":[{"message":{"content":"ready"}}]}`), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	}
}

func inactiveModelsBody(probe int32) string {
	switch {
	case probe == 1:
		return `{"object":"list","data":[]}`
	case probe < 4:
		return `{"object":"list","data":[{"id":"inactive"}]}`
	default:
		return `{"object":"list","data":[{"id":"koboldcpp/backend"}]}`
	}
}
