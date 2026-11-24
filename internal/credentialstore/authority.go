package credentialstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"tensors-router/internal/certgen"
)

type IssuedIdentity struct {
	CertificatePEM string
	PrivateKeyPEM  string
}

func (store *Store) CreateAuthority(ctx context.Context, name string) (Certificate, error) {
	commonName, err := normalizedName(name)
	if err != nil {
		return Certificate{}, err
	}
	keyPair, err := certgen.Authority(commonName, authorityLifetime)
	if err != nil {
		return Certificate{}, err
	}
	return store.ImportCertificate(ctx, CertificateImport{
		Name:           commonName,
		Kind:           KindAuthority,
		CertificatePEM: string(keyPair.CertificatePEM),
		PrivateKeyPEM:  string(keyPair.PrivateKeyPEM),
	})
}

func (store *Store) IssueIdentity(ctx context.Context, authorityReference string, name string, hosts []string) (IssuedIdentity, error) {
	subject, err := identitySubject(name, hosts)
	if err != nil {
		return IssuedIdentity{}, err
	}
	authority, err := store.authorityKeyPair(ctx, authorityReference)
	if err != nil {
		return IssuedIdentity{}, err
	}
	issued, err := certgen.Issue(authority, subject, identityLifetime, certgen.MutualTLSUsages)
	if err != nil {
		return IssuedIdentity{}, err
	}
	return IssuedIdentity{
		CertificatePEM: string(issued.CertificatePEM) + string(authority.CertificatePEM),
		PrivateKeyPEM:  string(issued.PrivateKeyPEM),
	}, nil
}

func (store *Store) authorityKeyPair(ctx context.Context, reference string) (certgen.KeyPair, error) {
	reference = strings.TrimSpace(reference)
	var certificatePEM, privateKeyPEM string
	err := store.handle.Reader().QueryRowContext(ctx, `SELECT certificate_pem, private_key_pem FROM credential_certificates
		WHERE (id = ? OR name = ?) AND kind = ?`, reference, reference, string(KindAuthority)).Scan(&certificatePEM, &privateKeyPEM)
	if errors.Is(err, sql.ErrNoRows) {
		return certgen.KeyPair{}, ErrNotFound
	}
	if err != nil {
		return certgen.KeyPair{}, err
	}
	certificates, err := certgen.ParseCertificates([]byte(certificatePEM))
	if err != nil {
		return certgen.KeyPair{}, err
	}
	privateKey, err := certgen.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		return certgen.KeyPair{}, err
	}
	return certgen.KeyPair{
		Certificate:    certificates[0],
		CertificatePEM: []byte(certificatePEM),
		PrivateKey:     privateKey,
		PrivateKeyPEM:  []byte(privateKeyPEM),
	}, nil
}

func (issued IssuedIdentity) AsIdentityImport(name string) CertificateImport {
	return CertificateImport{Name: name, Kind: KindIdentity, CertificatePEM: issued.CertificatePEM, PrivateKeyPEM: issued.PrivateKeyPEM}
}
