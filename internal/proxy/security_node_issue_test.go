package proxy

import (
	"context"
	"crypto"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"tensors-router/internal/certgen"
	"tensors-router/internal/cluster"
	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
)

type publicKeyHolder interface {
	Equal(crypto.PublicKey) bool
}

func TestSecurityRelayIssuesCertificateWhoseKeyOnlyTheNodeHolds(t *testing.T) {
	ctx := context.Background()
	testCluster := newSecurityTestCluster(t)
	authority, err := testCluster.masterLive.Store().CreateAuthority(ctx, "master-ca")
	if err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"authority_id": authority.ID, "kind": "api_certificate", "name": "worker-api", "hosts": []string{"worker.lan", "10.0.0.5"}, "activate": true}

	response := serveSecurityRequest(testCluster.master, http.MethodPost, masterRelayPath("issue"), jsonBody(t, request), "")
	if response.Code != http.StatusCreated {
		t.Fatalf("issue status %d: %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "PRIVATE KEY") {
		t.Fatalf("issue response carried a private key: %s", response.Body)
	}
	issued := decodeRecorded[credentialstore.Certificate](t, response.Body.Bytes())
	if issued.Kind != credentialstore.KindAPICertificate || issued.Source != credentialstore.SourceMaster || !issued.Active {
		t.Fatalf("issued certificate %#v", issued)
	}

	nodeSnapshot := testCluster.slaveLive.Current()
	if nodeSnapshot.APICertificate == nil || nodeSnapshot.APICertificateLeaf == nil {
		t.Fatal("node did not activate the issued API certificate")
	}
	nodeKey, ok := nodeSnapshot.APICertificate.PrivateKey.(crypto.Signer)
	if !ok {
		t.Fatalf("node API certificate key %T is not a signer", nodeSnapshot.APICertificate.PrivateKey)
	}
	if !nodeKey.Public().(publicKeyHolder).Equal(nodeSnapshot.APICertificateLeaf.PublicKey) {
		t.Fatal("installed certificate does not certify the node's own key")
	}
	if certgen.Fingerprint(nodeSnapshot.APICertificateLeaf) != issued.Fingerprint {
		t.Fatal("node serves another certificate than the one the master reported")
	}
	authorityChain, err := certgen.ParseCertificates([]byte(mustPublicPEM(t, testCluster.masterLive.Store(), authority.ID)))
	if err != nil {
		t.Fatal(err)
	}
	if err := nodeSnapshot.APICertificateLeaf.CheckSignatureFrom(authorityChain[0]); err != nil {
		t.Fatalf("installed certificate is not signed by the master authority: %v", err)
	}
	assertMasterHoldsNoIssuedKey(t, testCluster.masterLive.Store(), "worker-api")
	assertNoPendingKeys(t, testCluster.slaveLive.Store())

	inventory, err := testCluster.slave.localNodeInventory(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if state := inventory.APICertificate; state == nil || !state.Present || state.Source != credentialstore.SourceMaster || state.Name != "worker-api" {
		t.Fatalf("node inventory api certificate %#v", inventory.APICertificate)
	}
}

func TestSecurityRelayIssueDiscardsNodeKeyWhenSigningFails(t *testing.T) {
	testCluster := newSecurityTestCluster(t)
	request := map[string]any{"authority_id": "missing-ca", "kind": "identity", "name": "worker-id", "hosts": []string{"worker.lan"}}
	response := serveSecurityRequest(testCluster.master, http.MethodPost, masterRelayPath("issue"), jsonBody(t, request), "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("issue with unknown authority status %d: %s", response.Code, response.Body)
	}
	assertNoPendingKeys(t, testCluster.slaveLive.Store())
}

func TestSecurityRelayIssueRefusesToSignRequestForOtherHosts(t *testing.T) {
	ctx := context.Background()
	substituted, err := openTestCredentials(t).Store().CreatePendingKey(ctx, credentialstore.PendingKeyRequest{Name: "worker-id", Kind: credentialstore.KindIdentity, Hosts: []string{"master.lan"}})
	if err != nil {
		t.Fatal(err)
	}
	var callsMu sync.Mutex
	var calls []string
	dishonestNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callsMu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		callsMu.Unlock()
		if r.Method == http.MethodPost && r.URL.Path == securityNodePath+"/csr" {
			openai.WriteJSON(w, http.StatusCreated, substituted)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(dishonestNode.Close)
	masterLive := openTestCredentials(t)
	authority, err := masterLive.Store().CreateAuthority(ctx, "master-ca")
	if err != nil {
		t.Fatal(err)
	}
	master := newSecurityClusterService(t, cluster.RoleMaster, "master", masterLive, registryWithNode(t, "worker", dishonestNode.URL))

	request := map[string]any{"authority_id": authority.ID, "kind": "identity", "name": "worker-id", "hosts": []string{"worker.lan"}}
	response := serveSecurityRequest(master, http.MethodPost, masterRelayPath("issue"), jsonBody(t, request), "")
	if response.Code != http.StatusBadGateway {
		t.Fatalf("issue for substituted hosts status %d: %s", response.Code, response.Body)
	}
	wantCalls := []string{"POST " + securityNodePath + "/csr", "DELETE " + securityNodePath + "/csr/" + substituted.ID}
	callsMu.Lock()
	defer callsMu.Unlock()
	if strings.Join(calls, "|") != strings.Join(wantCalls, "|") {
		t.Fatalf("node calls %v, want %v", calls, wantCalls)
	}
}

func mustPublicPEM(t *testing.T, store *credentialstore.Store, reference string) string {
	t.Helper()
	certificatePEM, err := store.PublicCertificatePEM(context.Background(), reference)
	if err != nil {
		t.Fatal(err)
	}
	return certificatePEM
}

func assertMasterHoldsNoIssuedKey(t *testing.T, store *credentialstore.Store, name string) {
	t.Helper()
	certificates, err := store.Certificates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, certificate := range certificates {
		if certificate.Name == name {
			t.Fatalf("master stored the node certificate %#v", certificate)
		}
	}
	assertNoPendingKeys(t, store)
}

func assertNoPendingKeys(t *testing.T, store *credentialstore.Store) {
	t.Helper()
	pendingKeys, err := store.PendingKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingKeys) != 0 {
		t.Fatalf("store still holds pending keys %#v", pendingKeys)
	}
}
