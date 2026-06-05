package webui

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"testing"
)

func TestCertificateFilesGeneratesSelfSignedCertificate(t *testing.T) {
	cfg := ServerConfig{
		Bind:      "0.0.0.0:8443",
		StateDir:  t.TempDir(),
		CertHosts: []string{"webui.example.test", "172.81.90.24"},
	}
	certFile, keyFile, err := CertificateFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		t.Fatal(err)
	}
	cert := readCertificate(t, certFile)
	if cert.Subject.CommonName != "tensor-router-webui" {
		t.Fatalf("unexpected common name %q", cert.Subject.CommonName)
	}
	if !hasDNSName(cert, "localhost") || !hasDNSName(cert, "webui.example.test") {
		t.Fatalf("unexpected dns names %#v", cert.DNSNames)
	}
	if !hasIPAddress(cert, "127.0.0.1") || !hasIPAddress(cert, "::1") || !hasIPAddress(cert, "172.81.90.24") {
		t.Fatalf("unexpected ip addresses %#v", cert.IPAddresses)
	}
	if hasIPAddress(cert, "0.0.0.0") {
		t.Fatalf("wildcard bind address should not be a certificate identity")
	}
}

func TestSelfSignedCertificateNamesIncludesConcreteBindHost(t *testing.T) {
	dnsNames, ipAddresses := selfSignedCertificateNames(ServerConfig{Bind: "192.0.2.44:8443"})
	if !hasString(dnsNames, "localhost") {
		t.Fatalf("unexpected dns names %#v", dnsNames)
	}
	if !hasIP(ipAddresses, "192.0.2.44") {
		t.Fatalf("unexpected ip addresses %#v", ipAddresses)
	}
}

func TestCertificateFilesRefreshesGeneratedCertificateWhenNamesChange(t *testing.T) {
	stateDir := t.TempDir()
	cfg := ServerConfig{
		Bind:     "127.0.0.1:8443",
		StateDir: stateDir,
	}
	certFile, _, err := CertificateFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if hasDNSName(readCertificate(t, certFile), "webui.example.test") {
		t.Fatalf("unexpected dns name before config change")
	}
	cfg.CertHosts = []string{"webui.example.test"}
	certFile, _, err = CertificateFiles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDNSName(readCertificate(t, certFile), "webui.example.test") {
		t.Fatalf("expected refreshed certificate to include configured dns name")
	}
}

func readCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(content)
	if block == nil {
		t.Fatalf("certificate pem was not found")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func hasDNSName(cert *x509.Certificate, name string) bool {
	return hasString(cert.DNSNames, name)
}

func hasIPAddress(cert *x509.Certificate, value string) bool {
	return hasIP(cert.IPAddresses, value)
}

func hasString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func hasIP(values []net.IP, needle string) bool {
	parsed := net.ParseIP(needle)
	for _, value := range values {
		if value.Equal(parsed) {
			return true
		}
	}
	return false
}
