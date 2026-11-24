package webui

import (
	"crypto/tls"
	"os"
	"sync"
)

type ListenerCertificate struct {
	exportPath string
	fallback   tls.Certificate
	mu         sync.Mutex
	stamp      fileStamp
	exported   *tls.Certificate
}

func NewListenerCertificate(cfg Config) (*ListenerCertificate, error) {
	certFile, keyFile, err := CertificateFiles(cfg.Server)
	if err != nil {
		return nil, err
	}
	fallback, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	return &ListenerCertificate{exportPath: cfg.Router.APICertificateExport, fallback: fallback}, nil
}

func (certificate *ListenerCertificate) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return certificate.current(), nil
		},
	}
}

func (certificate *ListenerCertificate) current() *tls.Certificate {
	stamp := statFile(certificate.exportPath)
	certificate.mu.Lock()
	defer certificate.mu.Unlock()
	if stamp != certificate.stamp {
		certificate.stamp = stamp
		certificate.exported = loadExportedCertificate(certificate.exportPath, stamp)
	}
	if certificate.exported != nil {
		return certificate.exported
	}
	return &certificate.fallback
}

func loadExportedCertificate(path string, stamp fileStamp) *tls.Certificate {
	if !stamp.present {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	keyPair, err := tls.X509KeyPair(content, content)
	if err != nil {
		return nil
	}
	return &keyPair
}
