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

func TestPolicySeparatesInferenceAdminAndClusterCredentials(t *testing.T) {
	policy, err := NewPolicy(PolicyConfig{
		AllowedCIDRs:  []string{"127.0.0.0/8"},
		Profile:       ProfileSecure,
		InferenceKeys: []string{"inference"},
		AdminKeys:     []string{"admin"},
		ClusterToken:  "cluster",
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path   string
		token  string
		status int
	}{
		{"/v1/models", "inference", http.StatusNoContent},
		{"/v1/models", "admin", http.StatusUnauthorized},
		{"/router/v1/load", "admin", http.StatusNoContent},
		{"/router/v1/load", "inference", http.StatusUnauthorized},
		{"/router/v1/models", "admin", http.StatusNoContent},
		{"/router/v1/models", "inference", http.StatusUnauthorized},
		{"/router/v1/node/models", "cluster", http.StatusNoContent},
		{"/router/v1/node/models", "admin", http.StatusUnauthorized},
		{"/v1/models", "cluster", http.StatusUnauthorized},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
		request.RemoteAddr = "127.0.0.1:1234"
		request.Header.Set("Authorization", "Bearer "+testCase.token)
		policy.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(recorder, request)
		if recorder.Code != testCase.status {
			t.Fatalf("path %s token %s: got %d want %d", testCase.path, testCase.token, recorder.Code, testCase.status)
		}
	}
}

func TestTrustedLANSkipsPublicAuthButRequiresClusterToken(t *testing.T) {
	policy, err := NewPolicy(PolicyConfig{
		AllowedCIDRs: []string{"127.0.0.0/8"},
		Profile:      ProfileTrustedLAN,
		ClusterToken: "cluster",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := policy.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	publicRecorder := httptest.NewRecorder()
	publicRequest := httptest.NewRequest(http.MethodPost, "/router/v1/shutdown", nil)
	publicRequest.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(publicRecorder, publicRequest)
	if publicRecorder.Code != http.StatusNoContent {
		t.Fatalf("trusted LAN public route status %d", publicRecorder.Code)
	}
	clusterRecorder := httptest.NewRecorder()
	clusterRequest := httptest.NewRequest(http.MethodGet, "/router/v1/node/models", nil)
	clusterRequest.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(clusterRecorder, clusterRequest)
	if clusterRecorder.Code != http.StatusUnauthorized {
		t.Fatalf("trusted LAN cluster route status %d", clusterRecorder.Code)
	}
}
