package credentialstore

import (
	"context"
	"time"

	"tensors-router/internal/certgen"
)

type storedCertificate struct {
	id             string
	name           string
	kind           Kind
	certificatePEM string
	privateKeyPEM  string
	active         bool
	createdAt      time.Time
}

func (store *Store) storedCertificates(ctx context.Context) ([]storedCertificate, error) {
	rows, err := store.handle.Reader().QueryContext(ctx, `SELECT id, name, kind, certificate_pem, COALESCE(private_key_pem, ''), active, created_at
		FROM credential_certificates ORDER BY kind, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stored []storedCertificate
	for rows.Next() {
		var entry storedCertificate
		var createdAt int64
		if err := rows.Scan(&entry.id, &entry.name, &entry.kind, &entry.certificatePEM, &entry.privateKeyPEM, &entry.active, &createdAt); err != nil {
			return nil, err
		}
		entry.createdAt = time.Unix(createdAt, 0).UTC()
		stored = append(stored, entry)
	}
	return stored, rows.Err()
}

func describeCertificate(entry storedCertificate) (Certificate, error) {
	chain, err := certgen.ParseCertificates([]byte(entry.certificatePEM))
	if err != nil {
		return Certificate{}, err
	}
	leaf := chain[0]
	ipAddresses := make([]string, 0, len(leaf.IPAddresses))
	for _, ip := range leaf.IPAddresses {
		ipAddresses = append(ipAddresses, ip.String())
	}
	return Certificate{
		ID:          entry.id,
		Name:        entry.name,
		Kind:        entry.kind,
		Subject:     leaf.Subject.String(),
		Issuer:      leaf.Issuer.String(),
		DNSNames:    append([]string{}, leaf.DNSNames...),
		IPAddresses: ipAddresses,
		Fingerprint: certgen.Fingerprint(leaf),
		NotBefore:   leaf.NotBefore.UTC(),
		NotAfter:    leaf.NotAfter.UTC(),
		Active:      entry.active,
		CreatedAt:   entry.createdAt,
	}, nil
}
