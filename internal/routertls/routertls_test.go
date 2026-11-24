package routertls

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"tensors-router/internal/auth"
	"tensors-router/internal/credentialstore"
)

type testRouter struct {
	store   *credentialstore.Store
	live    *credentialstore.Live
	runtime *Runtime
	url     string
}

func newTestRouter(t *testing.T) *testRouter {
	t.Helper()
	ctx := context.Background()
	store, err := credentialstore.Open(ctx, credentialstore.Config{Path: filepath.Join(t.TempDir(), credentialstore.Filename)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	live, err := credentialstore.NewLive(ctx, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &testRouter{store: store, live: live, runtime: NewRuntime(live)}
}

func (router *testRouter) refresh(t *testing.T) {
	t.Helper()
	if err := router.live.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (router *testRouter) generateIdentity(t *testing.T, name string) credentialstore.Certificate {
	t.Helper()
	certificate, err := router.store.GenerateIdentity(context.Background(), name, []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := router.store.Activate(context.Background(), certificate.ID); err != nil {
		t.Fatal(err)
	}
	router.refresh(t)
	return certificate
}

func (router *testRouter) trust(t *testing.T, kind credentialstore.Kind, name string, certificatePEM string) {
	t.Helper()
	if _, err := router.store.ImportCertificate(context.Background(), credentialstore.CertificateImport{Name: name, Kind: kind, CertificatePEM: certificatePEM}); err != nil {
		t.Fatal(err)
	}
	router.refresh(t)
}

func (router *testRouter) publicPEM(t *testing.T, reference string) string {
	t.Helper()
	certificatePEM, err := router.store.PublicCertificatePEM(context.Background(), reference)
	if err != nil {
		t.Fatal(err)
	}
	return certificatePEM
}

func (router *testRouter) serve(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig:         router.runtime.ServerConfig(),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !auth.ClusterPeerAccepted(r) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() { _ = server.ServeTLS(listener, "", "") }()
	t.Cleanup(func() { _ = server.Close() })
	router.url = "https://" + listener.Addr().String() + "/router/v1/node/models"
}

func call(router *testRouter, target string) (*http.Response, error) {
	client := &http.Client{Transport: router.runtime.PeerTransport(), Timeout: 5 * time.Second}
	response, err := client.Get(target)
	if err == nil {
		_ = response.Body.Close()
	}
	return response, err
}

func TestPinnedSelfSignedPeersAuthenticateEachOther(t *testing.T) {
	server, client := newTestRouter(t), newTestRouter(t)
	server.generateIdentity(t, "server")
	client.generateIdentity(t, "client")
	server.trust(t, credentialstore.KindPinnedPeer, "client", client.publicPEM(t, "client"))
	client.trust(t, credentialstore.KindPinnedPeer, "server", server.publicPEM(t, "server"))
	server.serve(t)

	response, err := call(client, server.url)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("pinned peer was not accepted as verified: %d", response.StatusCode)
	}
}

func TestPeersIssuedByTrustedAuthorityAuthenticate(t *testing.T) {
	ctx := context.Background()
	authorityHolder, server, client := newTestRouter(t), newTestRouter(t), newTestRouter(t)
	authority, err := authorityHolder.store.CreateAuthority(ctx, "cluster-ca")
	if err != nil {
		t.Fatal(err)
	}
	for _, router := range []*testRouter{server, client} {
		issued, err := authorityHolder.store.IssueIdentity(ctx, authority.ID, "node", []string{"127.0.0.1"})
		if err != nil {
			t.Fatal(err)
		}
		installed, err := router.store.ImportCertificate(ctx, issued.AsIdentityImport("node"))
		if err != nil {
			t.Fatal(err)
		}
		if err := router.store.Activate(ctx, installed.ID); err != nil {
			t.Fatal(err)
		}
		router.trust(t, credentialstore.KindTrustedAuthority, "cluster-ca", authorityHolder.publicPEM(t, authority.ID))
	}
	server.serve(t)

	response, err := call(client, server.url)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("CA-issued peer was not accepted as verified: %d", response.StatusCode)
	}
}

func TestUntrustedClientCertificateIsNotAdmitted(t *testing.T) {
	server, stranger := newTestRouter(t), newTestRouter(t)
	server.generateIdentity(t, "server")
	stranger.generateIdentity(t, "stranger")
	stranger.trust(t, credentialstore.KindPinnedPeer, "server", server.publicPEM(t, "server"))
	server.serve(t)

	response, err := call(stranger, server.url)
	if err == nil && response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("server admitted a peer whose certificate it does not trust: %d", response.StatusCode)
	}
}

func TestClientWithoutCertificateIsNotAVerifiedPeer(t *testing.T) {
	server, anonymous := newTestRouter(t), newTestRouter(t)
	server.generateIdentity(t, "server")
	anonymous.trust(t, credentialstore.KindPinnedPeer, "server", server.publicPEM(t, "server"))
	server.serve(t)

	response, err := call(anonymous, server.url)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("client without certificate passed the peer check: %d", response.StatusCode)
	}
}

func TestClientRejectsServerOutsideTrustAnchors(t *testing.T) {
	server, client, other := newTestRouter(t), newTestRouter(t), newTestRouter(t)
	server.generateIdentity(t, "server")
	other.generateIdentity(t, "other")
	client.trust(t, credentialstore.KindPinnedPeer, "other", other.publicPEM(t, "other"))
	server.serve(t)

	if _, err := call(client, server.url); err == nil {
		t.Fatal("client trusted a server certificate outside its anchors")
	}
}

func TestRotatedIdentityIsServedOnNextHandshake(t *testing.T) {
	server := newTestRouter(t)
	first := server.generateIdentity(t, "first")
	server.serve(t)
	if served := servedFingerprint(t, server); served != first.Fingerprint {
		t.Fatalf("served %s, want first identity %s", served, first.Fingerprint)
	}
	second := server.generateIdentity(t, "second")
	if served := servedFingerprint(t, server); served != second.Fingerprint {
		t.Fatalf("served %s after rotation, want %s", served, second.Fingerprint)
	}
}

func servedFingerprint(t *testing.T, router *testRouter) string {
	t.Helper()
	client := &http.Client{Transport: router.runtime.PeerTransport(), Timeout: 5 * time.Second}
	response, err := client.Get(router.url)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	sum := sha256.Sum256(response.TLS.PeerCertificates[0].Raw)
	return hex.EncodeToString(sum[:])
}
