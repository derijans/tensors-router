package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"tensors-router/internal/credentialstore"
)

func jsonBody(t *testing.T, value any) string {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func decodeRecorded[T any](t *testing.T, body []byte) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return value
}

func TestSecuritySummaryReportsInstalledAPICertificateWithoutPrivateKeys(t *testing.T) {
	ctx := context.Background()
	routes, live := newSecurityTestRoutes(t)
	before := securityRequest(t, routes, http.MethodGet, securitySitePath+"/summary", "")
	if !strings.Contains(before.Body.String(), `"api_certificate":{"present":false}`) {
		t.Fatalf("summary without API certificate: %s", before.Body)
	}
	authority, err := live.Store().CreateAuthority(ctx, "site-ca")
	if err != nil {
		t.Fatal(err)
	}
	created := securityRequest(t, routes, http.MethodPost, securitySitePath+"/csr", `{"name":"api","kind":"api_certificate","hosts":["router.lan"]}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("csr status %d: %s", created.Code, created.Body)
	}
	pending := decodeRecorded[credentialstore.PendingKey](t, created.Body.Bytes())
	chainPEM, err := live.Store().SignCertificateRequest(ctx, authority.ID, credentialstore.KindAPICertificate, pending.RequestPEM)
	if err != nil {
		t.Fatal(err)
	}
	installed := securityRequest(t, routes, http.MethodPost, securitySitePath+"/install", jsonBody(t, map[string]any{"certificate_pem": chainPEM, "activate": true}))
	if installed.Code != http.StatusCreated {
		t.Fatalf("install status %d: %s", installed.Code, installed.Body)
	}
	certificate := decodeRecorded[credentialstore.Certificate](t, installed.Body.Bytes())
	if certificate.Source != credentialstore.SourceSupplied || !certificate.Active {
		t.Fatalf("installed certificate %#v", certificate)
	}
	if securityRequest(t, routes, http.MethodPost, securitySitePath+"/csr", `{"name":"later","kind":"identity"}`).Code != http.StatusCreated {
		t.Fatal("second csr was not created")
	}

	response := securityRequest(t, routes, http.MethodGet, securitySitePath+"/summary", "")
	if strings.Contains(response.Body.String(), "PRIVATE KEY") {
		t.Fatalf("summary leaked a private key: %s", response.Body)
	}
	summary := decodeRecorded[securitySummary](t, response.Body.Bytes())
	state := summary.APICertificate
	if !state.Present || state.Name != "api" || state.Source != credentialstore.SourceSupplied || state.Fingerprint != certificate.Fingerprint || state.NotAfter.IsZero() {
		t.Fatalf("api certificate state %#v", state)
	}
	if len(summary.PendingKeys) != 1 || summary.PendingKeys[0].Name != "later" || summary.NodeID != "local" {
		t.Fatalf("summary pending keys %#v node %q", summary.PendingKeys, summary.NodeID)
	}
}

func TestSecurityInstallRejectsCertificateSignedForAnotherKey(t *testing.T) {
	ctx := context.Background()
	routes, live := newSecurityTestRoutes(t)
	authority, err := live.Store().CreateAuthority(ctx, "site-ca")
	if err != nil {
		t.Fatal(err)
	}
	if code := securityRequest(t, routes, http.MethodPost, securitySitePath+"/csr", `{"name":"mine","kind":"identity","hosts":["router.lan"]}`).Code; code != http.StatusCreated {
		t.Fatalf("csr status %d", code)
	}
	foreign, err := openTestCredentials(t).Store().CreatePendingKey(ctx, credentialstore.PendingKeyRequest{Name: "mine", Kind: credentialstore.KindIdentity, Hosts: []string{"router.lan"}})
	if err != nil {
		t.Fatal(err)
	}
	chainPEM, err := live.Store().SignCertificateRequest(ctx, authority.ID, credentialstore.KindIdentity, foreign.RequestPEM)
	if err != nil {
		t.Fatal(err)
	}

	rejected := securityRequest(t, routes, http.MethodPost, securitySitePath+"/install", jsonBody(t, map[string]any{"certificate_pem": chainPEM}))
	if rejected.Code != http.StatusBadRequest || !strings.Contains(rejected.Body.String(), "does not match any pending key") {
		t.Fatalf("foreign certificate install status %d: %s", rejected.Code, rejected.Body)
	}
	pendingKeys, err := live.Store().PendingKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	certificates, err := live.Store().Certificates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingKeys) != 1 || len(certificates) != 1 {
		t.Fatalf("rejected install changed the store: pending=%d certificates=%d", len(pendingKeys), len(certificates))
	}
}
