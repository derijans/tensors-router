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
