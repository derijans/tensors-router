package auth

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"tensors-router/internal/credential"
)

type swappableKeys struct {
	mu   sync.Mutex
	keys credential.KeyHashes
}

func (source *swappableKeys) KeyHashes() credential.KeyHashes {
	source.mu.Lock()
	defer source.mu.Unlock()
	return source.keys
}

func (source *swappableKeys) set(keys credential.KeyHashes) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.keys = keys
}

func TestPolicyFollowsKeySourceWithoutRebuilding(t *testing.T) {
	source := &swappableKeys{keys: credential.KeyHashes{Admin: []credential.TokenHash{credential.HashToken("old-admin-key")}}}
	policy, err := NewPolicy(PolicyConfig{AllowedCIDRs: []string{"127.0.0.0/8"}, Profile: ProfileSecure, Keys: source})
	if err != nil {
		t.Fatal(err)
	}
	if got := statusFor(policy, "/router/v1/site/inventory", "old-admin-key"); got != http.StatusNoContent {
		t.Fatalf("old key before rotation: %d", got)
	}
	source.set(credential.KeyHashes{Admin: []credential.TokenHash{credential.HashToken("new-admin-key")}})
	if got := statusFor(policy, "/router/v1/site/inventory", "old-admin-key"); got != http.StatusUnauthorized {
		t.Fatalf("revoked key still accepted: %d", got)
	}
	if got := statusFor(policy, "/router/v1/site/inventory", "new-admin-key"); got != http.StatusNoContent {
		t.Fatalf("new key rejected: %d", got)
	}
}

func TestClusterRouteOverTLSRequiresVerifiedPeerCertificate(t *testing.T) {
	policy, err := NewPolicy(PolicyConfig{AllowedCIDRs: []string{"127.0.0.0/8"}, ClusterToken: "cluster-token-0123456789"})
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		state *tls.ConnectionState
		want  int
	}{
		"plain http keeps token-only behaviour": {nil, http.StatusNoContent},
		"tls without client certificate":        {&tls.ConnectionState{}, http.StatusUnauthorized},
		"tls with verified client certificate":  {&tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}}, http.StatusNoContent},
	}
	for name, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/router/v1/node/models", nil)
		request.RemoteAddr = "127.0.0.1:1234"
		request.TLS = testCase.state
		request.Header.Set("Authorization", "Bearer cluster-token-0123456789")
		policy.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(recorder, request)
		if recorder.Code != testCase.want {
			t.Fatalf("%s: status %d, want %d", name, recorder.Code, testCase.want)
		}
	}
}
