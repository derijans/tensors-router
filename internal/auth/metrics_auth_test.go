package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func metricsTestPolicy(t *testing.T, profile string) *Policy {
	t.Helper()
	policy, err := NewPolicy(PolicyConfig{
		AllowedCIDRs:  []string{"127.0.0.0/8"},
		Profile:       profile,
		InferenceKeys: []string{"inference-key"},
		AdminKeys:     []string{"admin-key"},
		MetricsKeys:   []string{"scrape-key"},
		ClusterToken:  "cluster-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func statusFor(policy *Policy, path string, bearer string) int {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.RemoteAddr = "127.0.0.1:1234"
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	policy.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)
	return recorder.Code
}

func TestMetricsRouteAcceptsOnlyScrapeAndAdminKeys(t *testing.T) {
	policy := metricsTestPolicy(t, ProfileSecure)
	for bearer, want := range map[string]int{
		"scrape-key":    http.StatusNoContent,
		"admin-key":     http.StatusNoContent,
		"inference-key": http.StatusUnauthorized,
		"cluster-token": http.StatusUnauthorized,
		"":              http.StatusUnauthorized,
	} {
		if got := statusFor(policy, MetricsPath, bearer); got != want {
			t.Fatalf("bearer %q on metrics: status=%d want=%d", bearer, got, want)
		}
	}
}

func TestScrapeKeyGrantsNoAdminAccess(t *testing.T) {
	policy := metricsTestPolicy(t, ProfileSecure)
	for _, path := range []string{"/router/v1/site/inventory", "/router/v1/metrics/extra", "/router/mcp"} {
		if got := statusFor(policy, path, "scrape-key"); got != http.StatusUnauthorized {
			t.Fatalf("scrape key on %s: status=%d, want 401", path, got)
		}
	}
	if got := statusFor(policy, "/v1/chat/completions", "scrape-key"); got != http.StatusUnauthorized {
		t.Fatalf("scrape key on inference: status=%d, want 401", got)
	}
	if principalForBearer(policy, "scrape-key").Admin {
		t.Fatal("scrape key resolved to an admin principal")
	}
}

func TestMetricsRouteFollowsTheTrustedLANProfile(t *testing.T) {
	policy := metricsTestPolicy(t, ProfileTrustedLAN)
	if got := statusFor(policy, MetricsPath, ""); got != http.StatusNoContent {
		t.Fatalf("trusted_lan metrics without key: status=%d, want 204", got)
	}
}

func TestMetricsRouteRequiresAdminKeyWhenNoScrapeKeyIsConfigured(t *testing.T) {
	policy, err := NewPolicy(PolicyConfig{AllowedCIDRs: []string{"127.0.0.0/8"}, Profile: ProfileSecure, AdminKeys: []string{"admin-key"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := statusFor(policy, MetricsPath, ""); got != http.StatusUnauthorized {
		t.Fatalf("metrics without key: status=%d, want 401", got)
	}
	if got := statusFor(policy, MetricsPath, "admin-key"); got != http.StatusNoContent {
		t.Fatalf("metrics with admin key: status=%d, want 204", got)
	}
}

func principalForBearer(policy *Policy, bearer string) Principal {
	return policy.principal("Bearer " + bearer)
}
