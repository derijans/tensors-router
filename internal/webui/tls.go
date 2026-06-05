package webui

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "tensor-router-webui",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	template.DNSNames, template.IPAddresses = selfSignedCertificateNames(cfg)
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return err
	}
	certOut, err := os.OpenFile(certFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		_ = certOut.Close()
		return err
	}
	if err := certOut.Close(); err != nil {
		return err
	}
	keyOut, err := os.OpenFile(keyFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		_ = keyOut.Close()
		return err
	}
	return keyOut.Close()
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
	if cert.Subject.CommonName != "tensor-router-webui" {
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
	block, _ := pem.Decode(content)
	if block == nil {
		return nil, x509.CertificateInvalidError{}
	}
	return x509.ParseCertificate(block.Bytes)
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
	values := []string{"localhost", "127.0.0.1", "::1"}
	if host := bindHost(cfg.Bind); isWildcardHost(host) {
		values = append(values, localInterfaceHosts()...)
	} else if host != "" {
		values = append(values, host)
	}
	values = append(values, cfg.CertHosts...)
	dnsNames := []string{}
	ipAddresses := []net.IP{}
	seenDNS := map[string]struct{}{}
	seenIP := map[string]struct{}{}
	for _, value := range values {
		value = unbracketHost(strings.TrimSpace(value))
		if value == "" || isWildcardHost(value) {
			continue
		}
		if host, _, err := net.SplitHostPort(value); err == nil {
			value = unbracketHost(host)
		}
		if ip := net.ParseIP(value); ip != nil {
			key := ip.String()
			if _, ok := seenIP[key]; !ok {
				seenIP[key] = struct{}{}
				ipAddresses = append(ipAddresses, ip)
			}
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seenDNS[key]; ok {
			continue
		}
		seenDNS[key] = struct{}{}
		dnsNames = append(dnsNames, value)
	}
	return dnsNames, ipAddresses
}

func localInterfaceHosts() []string {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	values := []string{}
	for _, address := range addresses {
		switch typedAddress := address.(type) {
		case *net.IPNet:
			if typedAddress.IP != nil {
				values = append(values, typedAddress.IP.String())
			}
		case *net.IPAddr:
			if typedAddress.IP != nil {
				values = append(values, typedAddress.IP.String())
			}
		}
	}
	return values
}

func bindHost(bind string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(bind))
	if err != nil {
		return unbracketHost(strings.TrimSpace(bind))
	}
	return unbracketHost(host)
}

func isWildcardHost(host string) bool {
	host = unbracketHost(strings.TrimSpace(host))
	return host == "" || host == "0.0.0.0" || host == "::"
}

func unbracketHost(host string) string {
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		return strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	return host
}
