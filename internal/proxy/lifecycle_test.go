package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/transportbody"
)

func TestControlRequestBodyLimitReturnsRequestTooLarge(t *testing.T) {
	service := NewService(ServiceConfig{MaxControlBodyBytes: 4})
	request := httptest.NewRequest(http.MethodPost, "/router/v1/load", strings.NewReader("12345"))
	recorder := httptest.NewRecorder()

	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge || !strings.Contains(recorder.Body.String(), `"type":"request_too_large"`) {
		t.Fatalf("unexpected response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestBeginDrainRejectsNewModelLoads(t *testing.T) {
	service := NewService(ServiceConfig{})
	service.BeginDrain()
	request := httptest.NewRequest(http.MethodPost, "/router/v1/load", strings.NewReader(`{"model":"a"}`))
	recorder := httptest.NewRecorder()

	service.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"type":"router_draining"`) {
		t.Fatalf("unexpected response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestNodeInferenceBodyIsNotTreatedAsControlPayload(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/router/v1/node/inference/v1/chat/completions", strings.NewReader("payload"))
	if controlRequestHasBody(request) {
		t.Fatal("cluster inference stream was classified as a control request")
	}
}

func TestKnownOversizedBackendResponseReturnsResponseTooLarge(t *testing.T) {
	service := NewService(ServiceConfig{TransportLimits: transportbody.Limits{MaxResponseBytes: 4}})
	response := &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"application/octet-stream"}},
		Body:          io.NopCloser(strings.NewReader("12345")),
		ContentLength: 5,
	}
	recorder := httptest.NewRecorder()

	if err := service.writeProxyResponse(recorder, response, "", false); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), `"type":"response_too_large"`) {
		t.Fatalf("unexpected response status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
