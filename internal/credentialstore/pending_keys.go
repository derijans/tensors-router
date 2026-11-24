package credentialstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"tensors-router/internal/certgen"
)

type PendingKey struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Kind        Kind      `json:"kind"`
	DNSNames    []string  `json:"dns_names"`
	IPAddresses []string  `json:"ip_addresses"`
	RequestPEM  string    `json:"request_pem"`
	CreatedAt   time.Time `json:"created_at"`
}

type PendingKeyRequest struct {
	Name  string
	Kind  Kind
	Hosts []string
}

func (store *Store) CreatePendingKey(ctx context.Context, request PendingKeyRequest) (PendingKey, error) {
	kind, err := ParseKind(string(request.Kind))
	if err != nil {
		return PendingKey{}, err
	}
	if !kind.Activatable() {
		return PendingKey{}, inputError("certificate requests are only made for identity and API certificates")
	}
	subject, err := identitySubject(request.Name, request.Hosts)
	if err != nil {
		return PendingKey{}, err
	}
	pending, err := certgen.NewCertificateRequest(subject)
	if err != nil {
		return PendingKey{}, err
	}
	publicKeyFingerprint, err := certgen.PublicKeyFingerprint(pending.PrivateKey.Public())
	if err != nil {
		return PendingKey{}, err
	}
	key := PendingKey{ID: newID(), Name: subject.CommonName, Kind: kind, RequestPEM: string(pending.RequestPEM), CreatedAt: store.now().UTC()}
	err = store.write(ctx, func(tx *sql.Tx) error {
		if err := ensureCredentialNameFree(ctx, tx, key.Name); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO credential_pending_keys (id, name, kind, request_pem, private_key_pem, public_key_fingerprint, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			key.ID, key.Name, string(key.Kind), key.RequestPEM, string(pending.PrivateKeyPEM), publicKeyFingerprint, key.CreatedAt.Unix())
		return err
	})
	if err != nil {
		return PendingKey{}, err
	}
	return describePendingKey(key)
}

func ensureCredentialNameFree(ctx context.Context, tx *sql.Tx, name string) error {
	taken, err := rowExists(ctx, tx, `SELECT 1 FROM credential_certificates WHERE name = ? UNION ALL SELECT 1 FROM credential_pending_keys WHERE name = ?`, name, name)
	if err != nil {
		return err
	}
	if taken {
		return ErrConflict
	}
	return nil
}

func (store *Store) PendingKeys(ctx context.Context) ([]PendingKey, error) {
	rows, err := store.handle.Reader().QueryContext(ctx, `SELECT id, name, kind, request_pem, created_at FROM credential_pending_keys ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []PendingKey{}
	for rows.Next() {
		var key PendingKey
		var createdAt int64
		if err := rows.Scan(&key.ID, &key.Name, &key.Kind, &key.RequestPEM, &createdAt); err != nil {
			return nil, err
		}
		key.CreatedAt = time.Unix(createdAt, 0).UTC()
		described, err := describePendingKey(key)
		if err != nil {
			return nil, err
		}
		keys = append(keys, described)
	}
	return keys, rows.Err()
}

func describePendingKey(key PendingKey) (PendingKey, error) {
	request, err := certgen.ParseCertificateRequest([]byte(key.RequestPEM))
	if err != nil {
		return PendingKey{}, err
	}
	key.DNSNames = append([]string{}, request.DNSNames...)
	key.IPAddresses = make([]string, 0, len(request.IPAddresses))
	for _, ip := range request.IPAddresses {
		key.IPAddresses = append(key.IPAddresses, ip.String())
	}
	return key, nil
}

func (store *Store) DeletePendingKey(ctx context.Context, reference string) error {
	reference = strings.TrimSpace(reference)
	return store.write(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `DELETE FROM credential_pending_keys WHERE id = ? OR name = ?`, reference, reference)
		if err != nil {
			return err
		}
		return requireAffected(result)
	})
}

type pendingKeyMaterial struct {
	id            string
	name          string
	kind          Kind
	privateKeyPEM string
}

func (store *Store) pendingKeyForPublicKey(ctx context.Context, publicKeyFingerprint string) (pendingKeyMaterial, error) {
	var material pendingKeyMaterial
	err := store.handle.Reader().QueryRowContext(ctx, `SELECT id, name, kind, private_key_pem FROM credential_pending_keys WHERE public_key_fingerprint = ?`,
		publicKeyFingerprint).Scan(&material.id, &material.name, &material.kind, &material.privateKeyPEM)
	if errors.Is(err, sql.ErrNoRows) {
		return pendingKeyMaterial{}, inputError("certificate does not match any pending key on this router; create a certificate request first and have exactly that request signed")
	}
	return material, err
}
