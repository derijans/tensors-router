package proxy

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteBackendFailureMapsUninitializedVLLM(t *testing.T) {
	recorder := httptest.NewRecorder()
	status := writeBackendFailure(recorder, errors.New("reload failed: companion error: backend_not_initialized"))
	if status != http.StatusServiceUnavailable || recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status return=%d response=%d", status, recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"type":"backend_not_initialized"`) || strings.Contains(body, "companion error") {
		t.Fatalf("unexpected response %s", body)
	}
}

func TestWriteBackendFailurePreservesGenericMapping(t *testing.T) {
	recorder := httptest.NewRecorder()
	status := writeBackendFailure(recorder, errors.New("runtime exited"))
	if status != http.StatusBadGateway || recorder.Code != http.StatusBadGateway {
		t.Fatalf("unexpected status return=%d response=%d", status, recorder.Code)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"type":"backend_error"`) || !strings.Contains(body, "runtime exited") {
		t.Fatalf("unexpected response %s", body)
	}
}
