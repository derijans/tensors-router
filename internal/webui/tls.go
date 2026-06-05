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
		return certFile, keyFile, nil
	}
	return certFile, keyFile, generateSelfSignedCertificate(certFile, keyFile)
}

func generateSelfSignedCertificate(certFile string, keyFile string) error {
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
			CommonName: "tensor-reuter-webui",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(5, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
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
