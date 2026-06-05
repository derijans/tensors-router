package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerRootServesIndexWithoutRedirect(t *testing.T) {
	server := NewServer(Config{}, nil, NewSessionManager("secret"))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	if location := recorder.Header().Get("Location"); location != "" {
		t.Fatalf("unexpected redirect to %q", location)
	}
}
