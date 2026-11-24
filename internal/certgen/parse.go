package certgen

import (
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
)

var errNoCertificate = errors.New("no PEM certificate found")

func ParseCertificates(content []byte) ([]*x509.Certificate, error) {
	var certificates []*x509.Certificate
	for rest := content; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != certificateBlockType {
			continue
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, certificate)
	}
	if len(certificates) == 0 {
		return nil, errNoCertificate
	}
	return certificates, nil
}

func ParsePrivateKey(content []byte) (crypto.Signer, error) {
	for rest := content; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return nil, errors.New("no PEM private key found")
		}
		switch block.Type {
		case privateKeyBlockType:
			return pkcs8Signer(block.Bytes)
		case "EC PRIVATE KEY":
			return x509.ParseECPrivateKey(block.Bytes)
		case "RSA PRIVATE KEY":
			return x509.ParsePKCS1PrivateKey(block.Bytes)
		}
	}
}

func pkcs8Signer(der []byte) (crypto.Signer, error) {
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("private key type %T cannot sign", key)
	}
	return signer, nil
}

func EncodeCertificates(certificates ...*x509.Certificate) []byte {
	var encoded []byte
	for _, certificate := range certificates {
		encoded = append(encoded, pem.EncodeToMemory(&pem.Block{Type: certificateBlockType, Bytes: certificate.Raw})...)
	}
	return encoded
}

func EncodePrivateKey(key crypto.Signer) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: privateKeyBlockType, Bytes: der}), nil
}

func Fingerprint(certificate *x509.Certificate) string {
	sum := sha256.Sum256(certificate.Raw)
	return hex.EncodeToString(sum[:])
}

func KeyMatchesCertificate(certificate *x509.Certificate, key crypto.Signer) bool {
	type comparablePublicKey interface {
		Equal(crypto.PublicKey) bool
	}
	public, ok := key.Public().(comparablePublicKey)
	return ok && public.Equal(certificate.PublicKey)
}
