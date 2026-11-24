package credentialstore

import (
	"context"
	"crypto/x509"

	"tensors-router/internal/certgen"
)

func (store *Store) SignCertificateRequest(ctx context.Context, authorityReference string, kind Kind, requestPEM string) (string, error) {
	usages, err := requestedUsages(kind)
	if err != nil {
		return "", err
	}
	request, err := certgen.ParseCertificateRequest([]byte(requestPEM))
	if err != nil {
		return "", inputError("certificate request: %v", err)
	}
	if len(request.DNSNames) == 0 && len(request.IPAddresses) == 0 {
		return "", inputError("certificate request must name at least one host name or IP address")
	}
	authority, err := store.authorityKeyPair(ctx, authorityReference)
	if err != nil {
		return "", err
	}
	issued, err := certgen.IssueForRequest(authority, request, identityLifetime, usages)
	if err != nil {
		return "", err
	}
	return string(certgen.EncodeCertificates(issued)) + string(authority.CertificatePEM), nil
}

func requestedUsages(kind Kind) ([]x509.ExtKeyUsage, error) {
	switch kind {
	case KindIdentity:
		return certgen.MutualTLSUsages, nil
	case KindAPICertificate:
		return certgen.ServerUsages, nil
	default:
		return nil, inputError("only identity and API certificate requests can be signed")
	}
}
