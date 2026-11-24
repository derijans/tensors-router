package credentialstore

import (
	"crypto"
	"crypto/x509"
	"slices"
	"strings"
	"time"

	"tensors-router/internal/certgen"
)

type CertificateImport struct {
	Name           string
	Kind           Kind
	CertificatePEM string
	PrivateKeyPEM  string
}

type validatedCertificate struct {
	name           string
	kind           Kind
	chain          []*x509.Certificate
	privateKey     crypto.Signer
	certificatePEM string
	privateKeyPEM  string
}

func validateImport(request CertificateImport, now time.Time) (validatedCertificate, error) {
	name, err := normalizedName(request.Name)
	if err != nil {
		return validatedCertificate{}, err
	}
	kind, err := ParseKind(string(request.Kind))
	if err != nil {
		return validatedCertificate{}, err
	}
	chain, err := certgen.ParseCertificates([]byte(request.CertificatePEM))
	if err != nil {
		return validatedCertificate{}, inputError("certificate: %v", err)
	}
	if now.After(chain[0].NotAfter) || now.Before(chain[0].NotBefore) {
		return validatedCertificate{}, inputError("certificate is outside its validity period (%s to %s)", chain[0].NotBefore.UTC().Format(time.RFC3339), chain[0].NotAfter.UTC().Format(time.RFC3339))
	}
	validated := validatedCertificate{name: name, kind: kind, chain: chain}
	if err := validated.attachPrivateKey(request.PrivateKeyPEM); err != nil {
		return validatedCertificate{}, err
	}
	if err := validated.checkKindRules(); err != nil {
		return validatedCertificate{}, err
	}
	validated.certificatePEM = string(certgen.EncodeCertificates(validated.storedChain()...))
	return validated, nil
}

func (validated *validatedCertificate) attachPrivateKey(privateKeyPEM string) error {
	keyRequired := kindHoldsPrivateKey(validated.kind)
	if strings.TrimSpace(privateKeyPEM) == "" {
		if keyRequired {
			return inputError("%s certificates require their private key", validated.kind)
		}
		return nil
	}
	if !keyRequired {
		return inputError("%s certificates must not include a private key", validated.kind)
	}
	key, err := certgen.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		return inputError("private key: %v", err)
	}
	if !certgen.KeyMatchesCertificate(validated.chain[0], key) {
		return inputError("private key does not match the certificate")
	}
	encoded, err := certgen.EncodePrivateKey(key)
	if err != nil {
		return err
	}
	validated.privateKey = key
	validated.privateKeyPEM = string(encoded)
	return nil
}

func (validated *validatedCertificate) checkKindRules() error {
	leaf := validated.chain[0]
	switch validated.kind {
	case KindIdentity, KindPinnedPeer:
		if leaf.IsCA {
			return inputError("%s certificates must be leaf certificates, not certificate authorities", validated.kind)
		}
		if !allowsMutualTLS(leaf) {
			return inputError("%s certificates must allow both server and client authentication", validated.kind)
		}
	case KindAuthority, KindTrustedAuthority:
		if !leaf.IsCA {
			return inputError("%s certificates must be certificate authorities", validated.kind)
		}
		if validated.kind == KindAuthority && leaf.KeyUsage&x509.KeyUsageCertSign == 0 {
			return inputError("authority certificates must allow certificate signing")
		}
	}
	return nil
}

func (validated *validatedCertificate) storedChain() []*x509.Certificate {
	if validated.kind == KindIdentity {
		return validated.chain
	}
	return validated.chain[:1]
}

func allowsMutualTLS(certificate *x509.Certificate) bool {
	usages := certificate.ExtKeyUsage
	if len(usages) == 0 || slices.Contains(usages, x509.ExtKeyUsageAny) {
		return true
	}
	return slices.Contains(usages, x509.ExtKeyUsageServerAuth) && slices.Contains(usages, x509.ExtKeyUsageClientAuth)
}

func kindHoldsPrivateKey(kind Kind) bool {
	return kind == KindIdentity || kind == KindAuthority
}
