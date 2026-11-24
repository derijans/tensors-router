package webui

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"tensors-router/internal/certgen"
)

func TestManagedRouterUsesHTTPSOnceTheRouterExportsATrustBundle(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), managedRouterTrustBundle)
	process := NewRouterProcess(RouterConfig{TrustBundleExport: bundle}, t.TempDir())
	if got := process.URL(); got != "http://"+managedRouterAddress {
		t.Fatalf("URL without bundle = %s", got)
	}
	if !slices.Contains(routerLaunchArguments(process.config), bundle) {
		t.Fatalf("managed router is not told where to export its bundle: %v", routerLaunchArguments(process.config))
	}
	if err := os.WriteFile(bundle, selfSignedPEM(t, "127.0.0.1").CertificatePEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := process.URL(); got != "https://"+managedRouterAddress {
		t.Fatalf("URL with bundle = %s", got)
	}
}

func TestRouterTrustAcceptsOnlyTheExportedCertificate(t *testing.T) {
	trusted := selfSignedPEM(t, "127.0.0.1")
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	certificate, err := tls.X509KeyPair(trusted.CertificatePEM, trusted.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()

	bundle := filepath.Join(t.TempDir(), managedRouterTrustBundle)
	client := &http.Client{Transport: newRouterTrust(bundle), Timeout: 5 * time.Second}
	if err := os.WriteFile(bundle, selfSignedPEM(t, "127.0.0.1").CertificatePEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get(server.URL); err == nil {
		t.Fatal("router certificate outside the bundle was trusted")
	}
	if err := os.WriteFile(bundle, trusted.CertificatePEM, 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(bundle, later, later); err != nil {
		t.Fatal(err)
	}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("exported router certificate was not trusted after the bundle changed: %v", err)
	}
	_ = response.Body.Close()
}

func selfSignedPEM(t *testing.T, host string) certgen.KeyPair {
	t.Helper()
	keyPair, err := certgen.SelfSigned(certgen.SubjectFromHosts("router", []string{host}), time.Hour, certgen.MutualTLSUsages)
	if err != nil {
		t.Fatal(err)
	}
	return keyPair
}

func startTLSServer(t *testing.T, keyPair certgen.KeyPair) *httptest.Server {
	t.Helper()
	certificate, err := tls.X509KeyPair(keyPair.CertificatePEM, keyPair.PrivateKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func TestManagedRouterTrustAcceptsPinnedAPICertificateWithoutLoopbackName(t *testing.T) {
	apiCertificate := selfSignedPEM(t, "api.example.com")
	server := startTLSServer(t, apiCertificate)
	bundle := filepath.Join(t.TempDir(), managedRouterTrustBundle)
	if err := os.WriteFile(bundle, apiCertificate.CertificatePEM, 0o600); err != nil {
		t.Fatal(err)
	}

	pinned := &http.Client{Transport: newManagedRouterTrust(bundle), Timeout: 5 * time.Second}
	response, err := pinned.Get(server.URL)
	if err != nil {
		t.Fatalf("managed router serving a pinned API certificate for another host name was refused: %v", err)
	}
	_ = response.Body.Close()

	external := &http.Client{Transport: newRouterTrust(bundle), Timeout: 5 * time.Second}
	if _, err := external.Get(server.URL); err == nil {
		t.Fatal("an external router must still match the host name of its URL")
	}
}

func TestManagedRouterTrustRefusesCertificateOutsideTheBundle(t *testing.T) {
	server := startTLSServer(t, selfSignedPEM(t, "api.example.com"))
	bundle := filepath.Join(t.TempDir(), managedRouterTrustBundle)
	if err := os.WriteFile(bundle, selfSignedPEM(t, "api.example.com").CertificatePEM, 0o600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: newManagedRouterTrust(bundle), Timeout: 5 * time.Second}
	if _, err := client.Get(server.URL); err == nil {
		t.Fatal("skipping the host name check also skipped chain verification")
	}
}
