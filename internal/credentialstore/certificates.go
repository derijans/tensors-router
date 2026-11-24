package credentialstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"tensors-router/internal/certgen"
)

const (
	identityLifetime  = 2 * 365 * 24 * time.Hour
	authorityLifetime = 10 * 365 * 24 * time.Hour
)

func (store *Store) ImportCertificate(ctx context.Context, request CertificateImport) (Certificate, error) {
	validated, err := validateImport(request, store.now())
	if err != nil {
		return Certificate{}, err
	}
	return store.insertCertificate(ctx, validated)
}

func (store *Store) GenerateIdentity(ctx context.Context, name string, hosts []string) (Certificate, error) {
	subject, err := identitySubject(name, hosts)
	if err != nil {
		return Certificate{}, err
	}
	keyPair, err := certgen.SelfSigned(subject, identityLifetime, certgen.MutualTLSUsages)
	if err != nil {
		return Certificate{}, err
	}
	return store.ImportCertificate(ctx, CertificateImport{
		Name:           name,
		Kind:           KindIdentity,
		CertificatePEM: string(keyPair.CertificatePEM),
		PrivateKeyPEM:  string(keyPair.PrivateKeyPEM),
	})
}

func identitySubject(name string, hosts []string) (certgen.Subject, error) {
	commonName, err := normalizedName(name)
	if err != nil {
		return certgen.Subject{}, err
	}
	subject := certgen.SubjectFromHosts(commonName, hosts)
	if len(subject.DNSNames) == 0 && len(subject.IPAddresses) == 0 {
		return certgen.Subject{}, inputError("at least one host name or IP address is required")
	}
	return subject, nil
}

func (store *Store) insertCertificate(ctx context.Context, validated validatedCertificate) (Certificate, error) {
	id := newID()
	fingerprint := certgen.Fingerprint(validated.chain[0])
	createdAt := store.now().UTC()
	err := store.write(ctx, func(tx *sql.Tx) error {
		conflict, err := rowExists(ctx, tx, `SELECT 1 FROM credential_certificates WHERE name = ? OR fingerprint = ?`, validated.name, fingerprint)
		if err != nil {
			return err
		}
		if conflict {
			return ErrConflict
		}
		var privateKey any
		if validated.privateKeyPEM != "" {
			privateKey = validated.privateKeyPEM
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO credential_certificates (id, name, kind, certificate_pem, private_key_pem, fingerprint, active, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
			id, validated.name, string(validated.kind), validated.certificatePEM, privateKey, fingerprint, createdAt.Unix())
		return err
	})
	if err != nil {
		return Certificate{}, err
	}
	return describeCertificate(storedCertificate{id: id, name: validated.name, kind: validated.kind, certificatePEM: validated.certificatePEM, createdAt: createdAt})
}

func (store *Store) Certificates(ctx context.Context) ([]Certificate, error) {
	stored, err := store.storedCertificates(ctx)
	if err != nil {
		return nil, err
	}
	certificates := make([]Certificate, 0, len(stored))
	for _, entry := range stored {
		certificate, err := describeCertificate(entry)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, certificate)
	}
	return certificates, nil
}

func (store *Store) Activate(ctx context.Context, reference string) error {
	return store.write(ctx, func(tx *sql.Tx) error {
		id, kind, err := resolveCertificate(ctx, tx, reference)
		if err != nil {
			return err
		}
		if kind != KindIdentity {
			return inputError("only identity certificates can be activated")
		}
		if _, err := tx.ExecContext(ctx, `UPDATE credential_certificates SET active = 0 WHERE active = 1`); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE credential_certificates SET active = 1 WHERE id = ?`, id)
		return err
	})
}

func (store *Store) DeleteCertificate(ctx context.Context, reference string, force bool) error {
	return store.write(ctx, func(tx *sql.Tx) error {
		id, _, err := resolveCertificate(ctx, tx, reference)
		if err != nil {
			return err
		}
		active, err := rowExists(ctx, tx, `SELECT 1 FROM credential_certificates WHERE id = ? AND active = 1`, id)
		if err != nil {
			return err
		}
		if active && !force {
			return inputError("certificate is the active router identity; activate another identity first or delete with force, which switches the router back to plain HTTP on restart")
		}
		result, err := tx.ExecContext(ctx, `DELETE FROM credential_certificates WHERE id = ?`, id)
		if err != nil {
			return err
		}
		return requireAffected(result)
	})
}

func resolveCertificate(ctx context.Context, tx *sql.Tx, reference string) (string, Kind, error) {
	reference = strings.TrimSpace(reference)
	var id string
	var kind Kind
	err := tx.QueryRowContext(ctx, `SELECT id, kind FROM credential_certificates WHERE id = ? OR name = ?`, reference, reference).Scan(&id, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return id, kind, err
}

func (store *Store) PublicCertificatePEM(ctx context.Context, reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	var certificatePEM string
	err := store.handle.Reader().QueryRowContext(ctx, `SELECT certificate_pem FROM credential_certificates WHERE id = ? OR name = ?`, reference, reference).Scan(&certificatePEM)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	certificates, err := certgen.ParseCertificates([]byte(certificatePEM))
	if err != nil {
		return "", err
	}
	return string(certgen.EncodeCertificates(certificates[0])), nil
}
