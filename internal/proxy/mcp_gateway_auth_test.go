package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tensors-router/internal/auth"
)

func TestTrustedLANReachesMCPOnlyWithAdminBearer(t *testing.T) {
	service, _ := newTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	policy, err := auth.NewPolicy(auth.PolicyConfig{
		AllowedCIDRs: []string{"127.0.0.0/8"},
		Profile:      auth.ProfileTrustedLAN,
		AdminKeys:    []string{"admin-key-long-enough-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := policy.Middleware(http.HandlerFunc(service.handlePublicMCP))

	tests := []struct {
		name          string
		authorization string
		want          int
	}{
		{name: "no bearer", want: http.StatusForbidden},
		{name: "wrong bearer", authorization: "Bearer inference-key-000001", want: http.StatusForbidden},
		{name: "admin bearer", authorization: "Bearer admin-key-long-enough-01", want: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/router/mcp", nil)
			request.RemoteAddr = "127.0.0.1:1234"
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != test.want {
				t.Fatalf("status %d, want %d: %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}
}
