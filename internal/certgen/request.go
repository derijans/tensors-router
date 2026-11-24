package certgen

import (
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

const certificateRequestBlockType = "CERTIFICATE REQUEST"

type PendingRequest struct {
	RequestPEM    []byte
	PrivateKey    crypto.Signer
	PrivateKeyPEM []byte
}

func NewCertificateRequest(subject Subject) (PendingRequest, error) {
	privateKey, err := newPrivateKey()
	if err != nil {
		return PendingRequest{}, err
	}
	template := &x509.CertificateRequest{
		Subject:     pkix.Name{CommonName: subject.CommonName},
		DNSNames:    subject.DNSNames,
		IPAddresses: subject.IPAddresses,
	}
	requestDER, err := x509.CreateCertificateRequest(rand.Reader, template, privateKey)
	if err != nil {
		return PendingRequest{}, err
	}
	privateKeyPEM, err := EncodePrivateKey(privateKey)
	if err != nil {
		return PendingRequest{}, err
	}
	return PendingRequest{
		RequestPEM:    pem.EncodeToMemory(&pem.Block{Type: certificateRequestBlockType, Bytes: requestDER}),
		PrivateKey:    privateKey,
		PrivateKeyPEM: privateKeyPEM,
	}, nil
}

func ParseCertificateRequest(content []byte) (*x509.CertificateRequest, error) {
	for rest := content; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, errors.New("no PEM certificate request found")
		}
		if block.Type != certificateRequestBlockType && block.Type != "NEW CERTIFICATE REQUEST" {
			continue
		}
		request, err := x509.ParseCertificateRequest(block.Bytes)
		if err != nil {
			return nil, err
		}
		if err := request.CheckSignature(); err != nil {
			return nil, fmt.Errorf("certificate request signature: %w", err)
		}
		return request, nil
	}
}

func IssueForRequest(authority KeyPair, request *x509.CertificateRequest, lifetime time.Duration, usages []x509.ExtKeyUsage) (*x509.Certificate, error) {
	if authority.Certificate == nil || authority.PrivateKey == nil || !authority.Certificate.IsCA {
		return nil, fmt.Errorf("issuing requires a certificate authority with its private key")
	}
	template, err := leafTemplate(Subject{
		CommonName:  request.Subject.CommonName,
		DNSNames:    request.DNSNames,
		IPAddresses: request.IPAddresses,
	}, lifetime, usages)
	if err != nil {
		return nil, err
	}
	if template.NotAfter.After(authority.Certificate.NotAfter) {
		template.NotAfter = authority.Certificate.NotAfter
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, authority.Certificate, request.PublicKey, authority.PrivateKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certificateDER)
}

func PublicKeyFingerprint(publicKey crypto.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}
