package proxy

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"tensors-router/internal/certgen"
	"tensors-router/internal/credentialstore"
)

func newLocalIssueTestRoutes(t *testing.T) (*routeTable, *credentialstore.Live) {
	t.Helper()
	live := openTestCredentials(t)
	admin := newSecurityAdmin(SecurityConfig{Credentials: live, DefaultIdentityHosts: []string{"127.0.0.1", "router.lan"}}, "local")
	return newRouteTable(func(http.ResponseWriter, *http.Request) bool { return true }, admin.routes()), live
}

func TestLocalIssueInstallsACertificateSignedByTheLocalAuthority(t *testing.T) {
	routes, live := newLocalIssueTestRoutes(t)
	authority, err := live.Store().CreateAuthority(context.Background(), "cluster-ca")
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"authority_id": authority.ID, "kind": "api_certificate", "name": "local-api", "activate": true}

	response := serveSecurityRequest(http.HandlerFunc(routes.serve), http.MethodPost, securityLocalIssuePath, jsonBody(t, request), "")
	if response.Code != http.StatusCreated {
		t.Fatalf("issue status %d: %s", response.Code, response.Body)
	}
	issued := decodeRecorded[credentialstore.Certificate](t, response.Body.Bytes())
	if issued.Kind != credentialstore.KindAPICertificate || issued.Source != credentialstore.SourceIssued || !issued.Active {
		t.Fatalf("issued certificate %#v", issued)
	}
	if !slices.Equal(issued.DNSNames, []string{"router.lan"}) || !slices.Equal(issued.IPAddresses, []string{"127.0.0.1"}) {
		t.Fatalf("issued hosts %v %v, want the router's default hosts", issued.DNSNames, issued.IPAddresses)
	}
	leaf := live.Current().APICertificateLeaf
	if leaf == nil || certgen.Fingerprint(leaf) != issued.Fingerprint {
		t.Fatal("router does not serve the issued API certificate")
	}
	authorityChain, err := certgen.ParseCertificates([]byte(mustPublicPEM(t, live.Store(), authority.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.CheckSignatureFrom(authorityChain[0]); err != nil {
		t.Fatalf("issued certificate is not signed by the local authority: %v", err)
	}
	assertNoPendingKeys(t, live.Store())
}

func TestLocalIssueDiscardsTheKeyWhenSigningFails(t *testing.T) {
	routes, live := newLocalIssueTestRoutes(t)
	request := map[string]any{"authority_id": "missing-ca", "kind": "identity", "name": "local-id"}
	if response := serveSecurityRequest(http.HandlerFunc(routes.serve), http.MethodPost, securityLocalIssuePath, jsonBody(t, request), ""); response.Code != http.StatusNotFound {
		t.Fatalf("issue with unknown authority: status %d: %s", response.Code, response.Body)
	}
	assertNoPendingKeys(t, live.Store())
}

func TestLocalIssueRefusesKindsThatCannotBeActivated(t *testing.T) {
	routes, live := newLocalIssueTestRoutes(t)
	request := map[string]any{"authority_id": "any", "kind": "authority", "name": "nested-ca"}
	if response := serveSecurityRequest(http.HandlerFunc(routes.serve), http.MethodPost, securityLocalIssuePath, jsonBody(t, request), ""); response.Code != http.StatusBadRequest {
		t.Fatalf("issue of an authority: status %d: %s", response.Code, response.Body)
	}
	assertNoPendingKeys(t, live.Store())
}

func TestSecuritySummaryReportsTheClusterSettings(t *testing.T) {
	settings := SecurityClusterSettings{
		Role:       "slave",
		ServerBind: "0.0.0.0:8080",
		Bind:       "0.0.0.0:8090",
		PublicURL:  "https://worker.lan:8090",
		MasterURL:  "https://master.lan:8090",
		SlaveURLs:  []string{},
	}
	admin := newSecurityAdmin(SecurityConfig{Credentials: openTestCredentials(t), Cluster: settings}, "worker")
	routes := newRouteTable(func(http.ResponseWriter, *http.Request) bool { return true }, admin.routes())
	response := serveSecurityRequest(http.HandlerFunc(routes.serve), http.MethodGet, securitySitePath+"/summary", "", "")
	summary := decodeRecorded[securitySummary](t, response.Body.Bytes())
	if summary.Cluster.Role != settings.Role || summary.Cluster.PublicURL != settings.PublicURL || summary.Cluster.Bind != settings.Bind ||
		summary.Cluster.MasterURL != settings.MasterURL || summary.Cluster.ServerBind != settings.ServerBind {
		t.Fatalf("summary cluster settings %#v", summary.Cluster)
	}
}
