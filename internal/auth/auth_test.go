package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuardAllowsLanWithoutBearer(t *testing.T) {
	guard, err := NewGuard([]string{"192.168.0.0/16"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "192.168.1.50:1234"

	guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
}

func TestGuardRejectsOutsideCIDR(t *testing.T) {
	guard, err := NewGuard([]string{"192.168.0.0/16"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "203.0.113.10:1234"

	guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
}

func TestGuardRequiresBearerWhenConfigured(t *testing.T) {
	guard, err := NewGuard([]string{"127.0.0.0/8"}, []string{"secret"})
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "127.0.0.1:1234"

	guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	request.Header.Set("Authorization", "Bearer secret")
	guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
}
