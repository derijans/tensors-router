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
	Source         string
}

func (request CertificateImport) source() string {
	if request.Source == "" {
		return SourceSupplied
	}
	return request.Source
}

type validatedCertificate struct {
	name           string
	source         string
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
	validated := validatedCertificate{name: name, kind: kind, chain: chain, source: request.source()}
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
	keyRequired := validated.kind.holdsPrivateKey()
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
	case KindAPICertificate:
		if leaf.IsCA {
			return inputError("API certificates must be leaf certificates, not certificate authorities")
		}
		if !allowsServerAuthentication(leaf) {
			return inputError("API certificates must allow server authentication")
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
	if validated.kind.Activatable() {
		return validated.chain
	}
	return validated.chain[:1]
}

func allowsMutualTLS(certificate *x509.Certificate) bool {
	return allowsUsage(certificate, x509.ExtKeyUsageServerAuth) && allowsUsage(certificate, x509.ExtKeyUsageClientAuth)
}

func allowsServerAuthentication(certificate *x509.Certificate) bool {
	return allowsUsage(certificate, x509.ExtKeyUsageServerAuth)
}

func allowsUsage(certificate *x509.Certificate, usage x509.ExtKeyUsage) bool {
	usages := certificate.ExtKeyUsage
	return len(usages) == 0 || slices.Contains(usages, x509.ExtKeyUsageAny) || slices.Contains(usages, usage)
}
