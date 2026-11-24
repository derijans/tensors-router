package webui

import (
	"crypto/x509"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tensors-router/internal/certgen"
)

const (
	generatedCertificateCommonName = "tensor-router-webui"
	generatedCertificateLifetime   = 5 * 365 * 24 * time.Hour
)

func CertificateFiles(cfg ServerConfig) (string, string, error) {
	if cfg.CertFile != "" && cfg.KeyFile != "" {
		return cfg.CertFile, cfg.KeyFile, nil
	}
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return "", "", err
	}
	certFile := filepath.Join(cfg.StateDir, "webui.crt")
	keyFile := filepath.Join(cfg.StateDir, "webui.key")
	if fileExists(certFile) && fileExists(keyFile) {
		if generatedCertificateReusable(certFile, cfg) {
			return certFile, keyFile, nil
		}
	}
	return certFile, keyFile, generateSelfSignedCertificate(certFile, keyFile, cfg)
}

func generateSelfSignedCertificate(certFile string, keyFile string, cfg ServerConfig) error {
	dnsNames, ipAddresses := selfSignedCertificateNames(cfg)
	subject := certgen.Subject{CommonName: generatedCertificateCommonName, DNSNames: dnsNames, IPAddresses: ipAddresses}
	keyPair, err := certgen.SelfSigned(subject, generatedCertificateLifetime, certgen.ServerUsages)
	if err != nil {
		return err
	}
	if err := os.WriteFile(certFile, keyPair.CertificatePEM, 0o600); err != nil {
		return err
	}
	return os.WriteFile(keyFile, keyPair.PrivateKeyPEM, 0o600)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func generatedCertificateReusable(certFile string, cfg ServerConfig) bool {
	cert, err := readCertificateFile(certFile)
	if err != nil {
		return true
	}
	if cert.Subject.CommonName != generatedCertificateCommonName {
		return !selfIssuedCertificate(cert)
	}
	dnsNames, ipAddresses := selfSignedCertificateNames(cfg)
	return certificateHasNames(cert, dnsNames, ipAddresses)
}

func selfIssuedCertificate(cert *x509.Certificate) bool {
	return cert.Subject.String() == cert.Issuer.String()
}

func readCertificateFile(certFile string) (*x509.Certificate, error) {
	content, err := os.ReadFile(certFile)
	if err != nil {
		return nil, err
	}
	certificates, err := certgen.ParseCertificates(content)
	if err != nil {
		return nil, err
	}
	return certificates[0], nil
}

func certificateHasNames(cert *x509.Certificate, dnsNames []string, ipAddresses []net.IP) bool {
	for _, name := range dnsNames {
		if !certificateHasDNSName(cert, name) {
			return false
		}
	}
	for _, ip := range ipAddresses {
		if !certificateHasIPAddress(cert, ip) {
			return false
		}
	}
	return true
}

func certificateHasDNSName(cert *x509.Certificate, name string) bool {
	for _, value := range cert.DNSNames {
		if strings.EqualFold(value, name) {
			return true
		}
	}
	return false
}

func certificateHasIPAddress(cert *x509.Certificate, ip net.IP) bool {
	for _, value := range cert.IPAddresses {
		if value.Equal(ip) {
			return true
		}
	}
	return false
}

func selfSignedCertificateNames(cfg ServerConfig) ([]string, []net.IP) {
	values := certgen.LoopbackHosts()
	if host := certgen.BindHost(cfg.Bind); certgen.IsWildcardHost(host) {
		values = append(values, certgen.LocalInterfaceHosts()...)
	} else {
		values = append(values, host)
	}
	values = append(values, cfg.CertHosts...)
	return certgen.SplitHosts(values)
}
