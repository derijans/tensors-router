package certgen

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"time"
)

const (
	certificateBlockType = "CERTIFICATE"
	privateKeyBlockType  = "PRIVATE KEY"
	clockSkewAllowance   = time.Hour
)

var MutualTLSUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}

var ServerUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}

type Subject struct {
	CommonName  string
	DNSNames    []string
	IPAddresses []net.IP
}

type KeyPair struct {
	Certificate    *x509.Certificate
	CertificatePEM []byte
	PrivateKey     crypto.Signer
	PrivateKeyPEM  []byte
}

func SelfSigned(subject Subject, lifetime time.Duration, usages []x509.ExtKeyUsage) (KeyPair, error) {
	privateKey, err := newPrivateKey()
	if err != nil {
		return KeyPair{}, err
	}
	template, err := leafTemplate(subject, lifetime, usages)
	if err != nil {
		return KeyPair{}, err
	}
	return sign(template, template, privateKey, privateKey)
}

func Authority(commonName string, lifetime time.Duration) (KeyPair, error) {
	privateKey, err := newPrivateKey()
	if err != nil {
		return KeyPair{}, err
	}
	template, err := baseTemplate(commonName, lifetime)
	if err != nil {
		return KeyPair{}, err
	}
	template.IsCA = true
	template.MaxPathLenZero = true
	template.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature
	return sign(template, template, privateKey, privateKey)
}

func Issue(authority KeyPair, subject Subject, lifetime time.Duration, usages []x509.ExtKeyUsage) (KeyPair, error) {
	if authority.Certificate == nil || authority.PrivateKey == nil || !authority.Certificate.IsCA {
		return KeyPair{}, fmt.Errorf("issuing requires a certificate authority with its private key")
	}
	privateKey, err := newPrivateKey()
	if err != nil {
		return KeyPair{}, err
	}
	template, err := leafTemplate(subject, lifetime, usages)
	if err != nil {
		return KeyPair{}, err
	}
	if template.NotAfter.After(authority.Certificate.NotAfter) {
		template.NotAfter = authority.Certificate.NotAfter
	}
	return sign(template, authority.Certificate, privateKey, authority.PrivateKey)
}

func newPrivateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

func baseTemplate(commonName string, lifetime time.Duration) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-clockSkewAllowance),
		NotAfter:              now.Add(lifetime),
		BasicConstraintsValid: true,
	}, nil
}

func leafTemplate(subject Subject, lifetime time.Duration, usages []x509.ExtKeyUsage) (*x509.Certificate, error) {
	template, err := baseTemplate(subject.CommonName, lifetime)
	if err != nil {
		return nil, err
	}
	template.KeyUsage = x509.KeyUsageDigitalSignature
	template.ExtKeyUsage = append([]x509.ExtKeyUsage(nil), usages...)
	template.DNSNames = append([]string(nil), subject.DNSNames...)
	template.IPAddresses = append([]net.IP(nil), subject.IPAddresses...)
	return template, nil
}

func sign(template *x509.Certificate, parent *x509.Certificate, subjectKey *ecdsa.PrivateKey, signerKey crypto.Signer) (KeyPair, error) {
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, parent, &subjectKey.PublicKey, signerKey)
	if err != nil {
		return KeyPair{}, err
	}
	certificate, err := x509.ParseCertificate(certificateDER)
	if err != nil {
		return KeyPair{}, err
	}
	privateKeyPEM, err := EncodePrivateKey(subjectKey)
	if err != nil {
		return KeyPair{}, err
	}
	return KeyPair{
		Certificate:    certificate,
		CertificatePEM: EncodeCertificates(certificate),
		PrivateKey:     subjectKey,
		PrivateKeyPEM:  privateKeyPEM,
	}, nil
}
