package proxy

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"tensors-router/internal/cluster"
	"tensors-router/internal/credentialstore"
)

func TestNodeSecurityRoutesRefuseCallsWithoutClusterToken(t *testing.T) {
	ctx := context.Background()
	live := openTestCredentials(t)
	node := newSecurityClusterService(t, cluster.RoleSlave, "worker", live, nil)
	calls := []struct {
		method string
		suffix string
		body   string
	}{
		{http.MethodGet, "summary", ""},
		{http.MethodPost, "csr", `{"name":"n","kind":"identity","hosts":["h"]}`},
		{http.MethodDelete, "csr/n", ""},
		{http.MethodPost, "install", `{"certificate_pem":""}`},
		{http.MethodPost, "trust", `{"name":"ca","kind":"trusted_authority","certificate_pem":""}`},
		{http.MethodPost, "certificates/generate", `{"name":"generated"}`},
		{http.MethodPost, "certificates/generated/activate", ""},
		{http.MethodDelete, "certificates/generated", ""},
		{http.MethodGet, "certificates/generated/pem", ""},
		{http.MethodPost, "tokens", `{"name":"t","role":"admin"}`},
		{http.MethodDelete, "tokens/t", ""},
		{http.MethodPost, "cluster-token", `{"name":"c","value":"0123456789abcdef0123456789abcdef"}`},
	}
	for _, call := range calls {
		path := securityNodePath + "/" + call.suffix
		for _, token := range []string{"", "wrong-token"} {
			if code := serveSecurityRequest(node, call.method, path, call.body, token).Code; code != http.StatusUnauthorized {
				t.Fatalf("%s %s with token %q status %d", call.method, path, token, code)
			}
		}
	}
	tokens, err := live.Store().Tokens(ctx)
	if err != nil {
		t.Fatal(err)
	}
	certificates, err := live.Store().Certificates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pendingKeys, err := live.Store().PendingKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tokens)+len(certificates)+len(pendingKeys) != 0 {
		t.Fatalf("refused calls changed the store: tokens=%d certificates=%d pending=%d", len(tokens), len(certificates), len(pendingKeys))
	}
	if code := serveSecurityRequest(node, http.MethodGet, securityNodePath+"/summary", "", securityTestClusterToken).Code; code != http.StatusOK {
		t.Fatalf("summary with cluster token status %d", code)
	}
}

func TestNodeSecurityTrustAcceptsOnlyPublicTrustAnchors(t *testing.T) {
	ctx := context.Background()
	live := openTestCredentials(t)
	node := newSecurityClusterService(t, cluster.RoleSlave, "worker", live, nil)
	authorityStore := openTestCredentials(t).Store()
	authority, err := authorityStore.CreateAuthority(ctx, "master-ca")
	if err != nil {
		t.Fatal(err)
	}
	authorityPEM, err := authorityStore.PublicCertificatePEM(ctx, authority.ID)
	if err != nil {
		t.Fatal(err)
	}
	trustPath := securityNodePath + "/trust"
	keyField := jsonBody(t, map[string]string{"name": "ca", "kind": "trusted_authority", "certificate_pem": authorityPEM, "private_key_pem": "-----BEGIN PRIVATE KEY-----"})
	keyInPEM := jsonBody(t, map[string]string{"name": "ca", "kind": "trusted_authority", "certificate_pem": authorityPEM + "-----BEGIN EC PRIVATE KEY-----\n"})
	keyedKind := jsonBody(t, map[string]string{"name": "ca", "kind": "identity", "certificate_pem": authorityPEM})
	for _, body := range []string{keyField, keyInPEM, keyedKind} {
		if response := serveSecurityRequest(node, http.MethodPost, trustPath, body, securityTestClusterToken); response.Code != http.StatusBadRequest {
			t.Fatalf("trust status %d for %s: %s", response.Code, body, response.Body)
		}
	}
	certificates, err := live.Store().Certificates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(certificates) != 0 {
		t.Fatalf("rejected trust requests stored %d certificates", len(certificates))
	}

	accepted := serveSecurityRequest(node, http.MethodPost, trustPath, jsonBody(t, map[string]string{"name": "ca", "kind": "trusted_authority", "certificate_pem": authorityPEM}), securityTestClusterToken)
	if accepted.Code != http.StatusCreated {
		t.Fatalf("public trust anchor status %d: %s", accepted.Code, accepted.Body)
	}
	trusted := decodeRecorded[credentialstore.Certificate](t, accepted.Body.Bytes())
	if trusted.Kind != credentialstore.KindTrustedAuthority || trusted.Fingerprint != authority.Fingerprint {
		t.Fatalf("trusted certificate %#v", trusted)
	}
}

func TestNodeSecurityInstallStoresMasterSourceAndRejectsKeyMaterial(t *testing.T) {
	ctx := context.Background()
	live := openTestCredentials(t)
	node := newSecurityClusterService(t, cluster.RoleSlave, "worker", live, nil)
	created := serveSecurityRequest(node, http.MethodPost, securityNodePath+"/csr", `{"name":"worker-id","kind":"identity","hosts":["worker.lan"]}`, securityTestClusterToken)
	if created.Code != http.StatusCreated {
		t.Fatalf("csr status %d: %s", created.Code, created.Body)
	}
	pending := decodeRecorded[credentialstore.PendingKey](t, created.Body.Bytes())
	authorityStore := openTestCredentials(t).Store()
	authority, err := authorityStore.CreateAuthority(ctx, "master-ca")
	if err != nil {
		t.Fatal(err)
	}
	chainPEM, err := authorityStore.SignCertificateRequest(ctx, authority.ID, credentialstore.KindIdentity, pending.RequestPEM)
	if err != nil {
		t.Fatal(err)
	}
	installPath := securityNodePath + "/install"
	withKey := serveSecurityRequest(node, http.MethodPost, installPath, jsonBody(t, map[string]any{"certificate_pem": chainPEM + "-----BEGIN PRIVATE KEY-----\n"}), securityTestClusterToken)
	if withKey.Code != http.StatusBadRequest || !strings.Contains(withKey.Body.String(), "private key") {
		t.Fatalf("install with key material status %d: %s", withKey.Code, withKey.Body)
	}
	installed := serveSecurityRequest(node, http.MethodPost, installPath, jsonBody(t, map[string]any{"certificate_pem": chainPEM, "activate": true}), securityTestClusterToken)
	if installed.Code != http.StatusCreated {
		t.Fatalf("install status %d: %s", installed.Code, installed.Body)
	}
	certificate := decodeRecorded[credentialstore.Certificate](t, installed.Body.Bytes())
	if certificate.Source != credentialstore.SourceMaster || !certificate.Active || !live.Current().TLSEnabled() {
		t.Fatalf("node install stored %#v, identity live=%v", certificate, live.Current().TLSEnabled())
	}
}
