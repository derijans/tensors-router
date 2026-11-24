package webui

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
)

func servedLeaf(t *testing.T, certificate *ListenerCertificate) *x509.Certificate {
	t.Helper()
	served, err := certificate.TLSConfig().GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(served.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf
}

func TestWebUIListenerServesTheExportedAPICertificateAndFallsBackWhenItIsRemoved(t *testing.T) {
	stateDir := t.TempDir()
	export := filepath.Join(stateDir, managedRouterAPICertificate)
	certificate, err := NewListenerCertificate(Config{
		Server: ServerConfig{Bind: "127.0.0.1:8443", StateDir: stateDir},
		Router: RouterConfig{APICertificateExport: export},
	})
	if err != nil {
		t.Fatal(err)
	}
	if leaf := servedLeaf(t, certificate); leaf.Subject.CommonName != generatedCertificateCommonName {
		t.Fatalf("served %q before any export; want the generated WebUI certificate", leaf.Subject.CommonName)
	}

	apiCertificate := selfSignedPEM(t, "api.example.com")
	if err := os.WriteFile(export, append(apiCertificate.CertificatePEM, apiCertificate.PrivateKeyPEM...), 0o600); err != nil {
		t.Fatal(err)
	}
	if leaf := servedLeaf(t, certificate); !leaf.Equal(apiCertificate.Certificate) {
		t.Fatalf("served %q after export; want the API certificate", leaf.Subject.CommonName)
	}

	if err := os.Remove(export); err != nil {
		t.Fatal(err)
	}
	if leaf := servedLeaf(t, certificate); leaf.Subject.CommonName != generatedCertificateCommonName {
		t.Fatalf("served %q after the export was removed; want the generated WebUI certificate", leaf.Subject.CommonName)
	}
}

func TestManagedWebUIHandsItsCertificateFilesToTheRouterOnce(t *testing.T) {
	stateDir := t.TempDir()
	content := "server:\n  state_dir: \"" + filepath.ToSlash(stateDir) + "\"\n  cert_file: \"webui.crt\"\n  key_file: \"webui.key\"\n"
	path := filepath.Join(t.TempDir(), "webui.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	arguments := routerLaunchArguments(cfg.Router)
	want := []string{"--import-api-cert", cfg.Server.CertFile, "--import-api-key", cfg.Server.KeyFile, "--api-certificate-export"}
	for _, argument := range want {
		if !containsArgument(arguments, argument) {
			t.Fatalf("router launch arguments %v miss %s", arguments, argument)
		}
	}
}

func containsArgument(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted {
			return true
		}
	}
	return false
}
