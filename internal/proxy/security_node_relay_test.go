package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"tensors-router/internal/cluster"
	"tensors-router/internal/credentialstore"
)

func TestSecurityRelayForwardsNodeSummaryAndNodeErrors(t *testing.T) {
	testCluster := newSecurityTestCluster(t)
	master := testCluster.master

	summaryResponse := serveSecurityRequest(master, http.MethodGet, masterRelayPath("summary"), "", "")
	if summaryResponse.Code != http.StatusOK {
		t.Fatalf("relayed summary status %d: %s", summaryResponse.Code, summaryResponse.Body)
	}
	if summary := decodeRecorded[securitySummary](t, summaryResponse.Body.Bytes()); summary.NodeID != "worker" {
		t.Fatalf("relayed summary came from %q", summary.NodeID)
	}

	invalidRole := serveSecurityRequest(master, http.MethodPost, masterRelayPath("tokens"), `{"name":"x","role":"root"}`, "")
	if invalidRole.Code != http.StatusBadRequest || !strings.Contains(invalidRole.Body.String(), "role must be one of") {
		t.Fatalf("relayed invalid role status %d: %s", invalidRole.Code, invalidRole.Body)
	}
	missingToken := serveSecurityRequest(master, http.MethodDelete, masterRelayPath("tokens/missing"), "", "")
	if missingToken.Code != http.StatusNotFound || !strings.Contains(missingToken.Body.String(), credentialstore.ErrNotFound.Error()) {
		t.Fatalf("relayed missing token status %d: %s", missingToken.Code, missingToken.Body)
	}
	if code := serveSecurityRequest(master, http.MethodGet, masterRelayPath("tokens/anything/secret"), "", "").Code; code != http.StatusNotFound {
		t.Fatalf("non-allowlisted relay status %d", code)
	}
	if code := serveSecurityRequest(master, http.MethodGet, securityNodesPath+"missing/summary", "", "").Code; code != http.StatusNotFound {
		t.Fatalf("unknown node status %d", code)
	}
}

func TestSecurityRelayForwardsForceDeletionAndPublicPEM(t *testing.T) {
	testCluster := newSecurityTestCluster(t)
	master := testCluster.master
	generated := serveSecurityRequest(master, http.MethodPost, masterRelayPath("certificates/generate"), `{"name":"worker-id","activate":true}`, "")
	if generated.Code != http.StatusCreated {
		t.Fatalf("relayed generate status %d: %s", generated.Code, generated.Body)
	}
	certificate := decodeRecorded[credentialstore.Certificate](t, generated.Body.Bytes())

	pem := serveSecurityRequest(master, http.MethodGet, masterRelayPath("certificates/"+certificate.ID+"/pem"), "", "")
	if pem.Code != http.StatusOK || pem.Header().Get("Content-Type") != "application/x-pem-file" || !strings.HasPrefix(pem.Body.String(), "-----BEGIN CERTIFICATE-----") || strings.Contains(pem.Body.String(), "PRIVATE KEY") {
		t.Fatalf("relayed pem status %d type %q: %s", pem.Code, pem.Header().Get("Content-Type"), pem.Body)
	}

	activePath := masterRelayPath("certificates/" + certificate.ID)
	if code := serveSecurityRequest(master, http.MethodDelete, activePath, "", "").Code; code != http.StatusBadRequest {
		t.Fatalf("relayed delete of active identity without force status %d", code)
	}
	if code := serveSecurityRequest(master, http.MethodDelete, activePath+"?force=true", "", "").Code; code != http.StatusNoContent {
		t.Fatalf("relayed forced delete status %d", code)
	}
	remaining, err := testCluster.slaveLive.Store().Certificates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("forced delete left %d certificates on the node", len(remaining))
	}
}

func TestSecurityRelayReportsUnreachableNode(t *testing.T) {
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()
	master := newSecurityClusterService(t, cluster.RoleMaster, "master", openTestCredentials(t), registryWithNode(t, "worker", closed.URL))
	response := serveSecurityRequest(master, http.MethodGet, masterRelayPath("summary"), "", "")
	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "node_unreachable") {
		t.Fatalf("unreachable node status %d: %s", response.Code, response.Body)
	}
}

func TestSecurityRelayIsOnlyServedByMaster(t *testing.T) {
	testCluster := newSecurityTestCluster(t)
	registry := registryWithNode(t, "worker", testCluster.slaveServer.URL)
	for _, role := range []string{cluster.RoleStandalone, cluster.RoleSlave} {
		router := newSecurityClusterService(t, role, "other", openTestCredentials(t), registry)
		if code := serveSecurityRequest(router, http.MethodGet, masterRelayPath("summary"), "", "").Code; code != http.StatusNotFound {
			t.Fatalf("%s relay status %d", role, code)
		}
	}
	if code := serveSecurityRequest(testCluster.master, http.MethodGet, masterRelayPath("summary"), "", "").Code; code != http.StatusOK {
		t.Fatalf("master relay status %d", code)
	}
}

func TestSecurityRelayTrustSendsMasterAuthorityByID(t *testing.T) {
	ctx := context.Background()
	testCluster := newSecurityTestCluster(t)
	authority, err := testCluster.masterLive.Store().CreateAuthority(ctx, "master-ca")
	if err != nil {
		t.Fatal(err)
	}
	response := serveSecurityRequest(testCluster.master, http.MethodPost, masterRelayPath("trust"), jsonBody(t, map[string]string{"certificate_id": authority.ID}), "")
	if response.Code != http.StatusCreated {
		t.Fatalf("relayed trust status %d: %s", response.Code, response.Body)
	}
	trusted := decodeRecorded[credentialstore.Certificate](t, response.Body.Bytes())
	if trusted.Kind != credentialstore.KindTrustedAuthority || trusted.Fingerprint != authority.Fingerprint || trusted.Name != "master-ca" {
		t.Fatalf("node trusted %#v", trusted)
	}
	identity, err := testCluster.masterLive.Store().GenerateIdentity(ctx, "master-id", []string{"master.lan"})
	if err != nil {
		t.Fatal(err)
	}
	if code := serveSecurityRequest(testCluster.master, http.MethodPost, masterRelayPath("trust"), jsonBody(t, map[string]string{"certificate_id": identity.ID}), "").Code; code != http.StatusBadRequest {
		t.Fatalf("trusting a master identity status %d", code)
	}
}

func TestSecurityRelayPushesMasterClusterToken(t *testing.T) {
	ctx := context.Background()
	testCluster := newSecurityTestCluster(t)
	if code := serveSecurityRequest(testCluster.master, http.MethodPost, masterRelayPath("cluster-token"), "", "").Code; code != http.StatusBadRequest {
		t.Fatalf("push without a master cluster token status %d", code)
	}
	_, secret, err := testCluster.masterLive.Store().CreateToken(ctx, credentialstore.NewToken{Name: "cluster", Role: credentialstore.RoleCluster})
	if err != nil {
		t.Fatal(err)
	}
	if err := testCluster.masterLive.Refresh(ctx); err != nil {
		t.Fatal(err)
	}

	first := serveSecurityRequest(testCluster.master, http.MethodPost, masterRelayPath("cluster-token"), "", "")
	if first.Code != http.StatusCreated || strings.Contains(first.Body.String(), secret) {
		t.Fatalf("first push status %d: %s", first.Code, first.Body)
	}
	if !testCluster.slaveLive.AcceptsClusterToken(secret) {
		t.Fatal("node does not accept the pushed cluster token")
	}
	if code := serveSecurityRequest(testCluster.master, http.MethodPost, masterRelayPath("cluster-token"), `{"name":"again"}`, "").Code; code != http.StatusOK {
		t.Fatalf("repeated push status %d", code)
	}
}
