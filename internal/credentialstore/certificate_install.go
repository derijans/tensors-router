package credentialstore

import (
	"context"
	"database/sql"

	"tensors-router/internal/certgen"
)

type SignedCertificateInstall struct {
	CertificatePEM string
	Activate       bool
	Source         string
}

func (store *Store) InstallSignedCertificate(ctx context.Context, install SignedCertificateInstall) (Certificate, error) {
	chain, err := certgen.ParseCertificates([]byte(install.CertificatePEM))
	if err != nil {
		return Certificate{}, inputError("certificate: %v", err)
	}
	publicKeyFingerprint, err := certgen.PublicKeyFingerprint(chain[0].PublicKey)
	if err != nil {
		return Certificate{}, inputError("certificate public key: %v", err)
	}
	pending, err := store.pendingKeyForPublicKey(ctx, publicKeyFingerprint)
	if err != nil {
		return Certificate{}, err
	}
	validated, err := validateImport(CertificateImport{
		Name:           pending.name,
		Kind:           pending.kind,
		CertificatePEM: install.CertificatePEM,
		PrivateKeyPEM:  pending.privateKeyPEM,
		Source:         install.Source,
	}, store.now())
	if err != nil {
		return Certificate{}, err
	}
	var inserted storedCertificate
	err = store.write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM credential_pending_keys WHERE id = ?`, pending.id)
		if err != nil {
			return err
		}
		if err := requireAffected(result); err != nil {
			return err
		}
		inserted, err = store.insertValidatedCertificate(ctx, tx, validated)
		if err != nil || !install.Activate {
			return err
		}
		inserted.active = true
		return activateCertificate(ctx, tx, inserted.id, inserted.kind)
	})
	if err != nil {
		return Certificate{}, err
	}
	return describeCertificate(inserted)
}
