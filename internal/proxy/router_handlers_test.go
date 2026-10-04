package proxy

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNodeInferencePathAcceptsSupportedRoutesAndRejectsExternalPaths(t *testing.T) {
	for _, requestPath := range []string{
		"/router/v1/node/inference/v1/chat/completions",
		"/router/v1/node/inference/v1/images/generations",
		"/router/v1/node/inference/v1/audio/speech",
		"/router/v1/node/inference/musicui/",
	} {
		if _, ok := nodeInferencePath(requestPath); !ok {
			t.Fatalf("supported inference route %q was rejected", requestPath)
		}
	}

	for _, requestPath := range []string{
		"/router/v1/node/inference//attacker.example/v1/chat/completions",
		"/router/v1/node/inference/\\attacker.example/v1/chat/completions",
		"/router/v1/node/inference/unknown",
	} {
		if _, ok := nodeInferencePath(requestPath); ok {
			t.Fatalf("unsafe inference route %q was accepted", requestPath)
		}
	}
}

func TestRouterLoadRejectsUnknownModelAsNotFound(t *testing.T) {
	service, _ := newTestService(t, http.NotFoundHandler())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/load", strings.NewReader(`{"model":"missing"}`))
	request.Header.Set("Content-Type", "application/json")
	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), `"type":"invalid_request_error"`) {
		t.Fatalf("unknown model load status %d body %s", recorder.Code, recorder.Body.String())
	}
}
