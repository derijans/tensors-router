package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/credentialstore"
)

func newSecurityTestRoutes(t *testing.T) (*routeTable, *credentialstore.Live) {
	t.Helper()
	live := openTestCredentials(t)
	plainSharedListener := SecurityListener{Name: "shared", Bind: "127.0.0.1:8080", WantsTLS: func() bool { return live.Current().TLSEnabled() }}
	admin := newSecurityAdmin(SecurityConfig{Credentials: live, DefaultIdentityHosts: []string{"127.0.0.1"}, Listeners: []SecurityListener{plainSharedListener}}, "local")
	return newRouteTable(func(http.ResponseWriter, *http.Request) bool { return true }, admin.routes()), live
}

func securityRequest(t *testing.T, routes *routeTable, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	routes.serve(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
	return recorder
}

func TestSecurityAPIShowsTokenSecretOnlyAtCreation(t *testing.T) {
	routes, live := newSecurityTestRoutes(t)
	created := securityRequest(t, routes, http.MethodPost, securityTokensPath, `{"name":"ops","role":"ADMIN"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", created.Code, created.Body)
	}
	var response securityTokenCreatedResponse
	if err := json.Unmarshal(created.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Secret == "" || response.Token.Role != credentialstore.RoleAdmin {
		t.Fatalf("unexpected create response %#v", response)
	}
	if len(live.Current().Keys.Admin) != 1 {
		t.Fatal("new token did not reach the live snapshot")
	}
	listed := securityRequest(t, routes, http.MethodGet, securityTokensPath, "")
	if strings.Contains(listed.Body.String(), response.Secret) {
		t.Fatal("token list leaked the secret")
	}
	revealed := securityRequest(t, routes, http.MethodGet, securityTokensPath+"/"+response.Token.ID+"/secret", "")
	if revealed.Code != http.StatusBadRequest {
		t.Fatalf("admin secret reveal status %d", revealed.Code)
	}
}

func TestSecurityAPINeverReturnsPrivateKeysOfStoredCertificates(t *testing.T) {
	routes, live := newSecurityTestRoutes(t)
	generated := securityRequest(t, routes, http.MethodPost, securityCertificatesPath+"/generate", `{"name":"node1","activate":true}`)
	if generated.Code != http.StatusCreated {
		t.Fatalf("generate status %d: %s", generated.Code, generated.Body)
	}
	authority := securityRequest(t, routes, http.MethodPost, securityAuthoritiesPath, `{"name":"cluster-ca"}`)
	if authority.Code != http.StatusCreated {
		t.Fatalf("authority status %d: %s", authority.Code, authority.Body)
	}
	if !live.Current().TLSEnabled() {
		t.Fatal("activated identity did not reach the live snapshot")
	}
	var certificate credentialstore.Certificate
	if err := json.Unmarshal(authority.Body.Bytes(), &certificate); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{securityCertificatesPath, securityCertificatesPath + "/" + certificate.ID + "/pem"} {
		body := securityRequest(t, routes, http.MethodGet, path, "").Body.String()
		if strings.Contains(body, "PRIVATE KEY") {
			t.Fatalf("%s leaked a private key", path)
		}
	}
	status := securityRequest(t, routes, http.MethodGet, securityStatusPath, "").Body.String()
	if !strings.Contains(status, `"restart_required":true`) {
		t.Fatalf("plain listener with active identity did not ask for restart: %s", status)
	}
}

func TestSecurityAPIMapsStoreErrorsToClientStatuses(t *testing.T) {
	routes, _ := newSecurityTestRoutes(t)
	if code := securityRequest(t, routes, http.MethodPost, securityTokensPath, `{"name":"x","role":"root"}`).Code; code != http.StatusBadRequest {
		t.Fatalf("invalid role status %d", code)
	}
	if code := securityRequest(t, routes, http.MethodDelete, securityTokensPath+"/missing", "").Code; code != http.StatusNotFound {
		t.Fatalf("missing token status %d", code)
	}
	securityRequest(t, routes, http.MethodPost, securityTokensPath, `{"name":"dup","role":"inference"}`)
	if code := securityRequest(t, routes, http.MethodPost, securityTokensPath, `{"name":"dup","role":"inference"}`).Code; code != http.StatusConflict {
		t.Fatalf("duplicate name status %d", code)
	}
}
