package credentialstore

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"

	"tensors-router/internal/certgen"
	"tensors-router/internal/credential"
)

type Snapshot struct {
	Revision             int64
	Keys                 credential.KeyHashes
	OutboundClusterToken string
	Identity             *tls.Certificate
	TrustAnchors         []*x509.Certificate
}

func (snapshot *Snapshot) TLSEnabled() bool {
	return snapshot != nil && snapshot.Identity != nil
}

func (store *Store) Snapshot(ctx context.Context) (*Snapshot, error) {
	revision, err := store.Revision(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{Revision: revision}
	if err := store.loadKeys(ctx, snapshot); err != nil {
		return nil, err
	}
	if err := store.loadCertificates(ctx, snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (store *Store) loadKeys(ctx context.Context, snapshot *Snapshot) error {
	rows, err := store.handle.Reader().QueryContext(ctx, `SELECT role, secret_hash, COALESCE(cluster_secret, '')
		FROM credential_tokens ORDER BY created_at, rowid`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var role Role
		var hashBytes []byte
		var clusterSecret string
		if err := rows.Scan(&role, &hashBytes, &clusterSecret); err != nil {
			return err
		}
		var hash credential.TokenHash
		if len(hashBytes) != len(hash) {
			return errors.New("credential token hash has an unexpected length")
		}
		copy(hash[:], hashBytes)
		appendKey(&snapshot.Keys, role, hash)
		if role == RoleCluster {
			snapshot.OutboundClusterToken = clusterSecret
		}
	}
	return rows.Err()
}

func appendKey(keys *credential.KeyHashes, role Role, hash credential.TokenHash) {
	switch role {
	case RoleInference:
		keys.Inference = append(keys.Inference, hash)
	case RoleAdmin:
		keys.Admin = append(keys.Admin, hash)
	case RoleMetrics:
		keys.Metrics = append(keys.Metrics, hash)
	case RoleCluster:
		keys.Cluster = append(keys.Cluster, hash)
	}
}

func (store *Store) loadCertificates(ctx context.Context, snapshot *Snapshot) error {
	stored, err := store.storedCertificates(ctx)
	if err != nil {
		return err
	}
	for _, entry := range stored {
		chain, err := certgen.ParseCertificates([]byte(entry.certificatePEM))
		if err != nil {
			return err
		}
		if entry.kind != KindIdentity {
			snapshot.TrustAnchors = append(snapshot.TrustAnchors, chain[0])
			continue
		}
		if !entry.active {
			continue
		}
		identity, err := tls.X509KeyPair([]byte(entry.certificatePEM), []byte(entry.privateKeyPEM))
		if err != nil {
			return err
		}
		snapshot.Identity = &identity
		snapshot.TrustAnchors = append(snapshot.TrustAnchors, chain[0])
	}
	return nil
}

func (store *Store) TrustBundle(ctx context.Context) ([]byte, error) {
	snapshot, err := store.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return certgen.EncodeCertificates(snapshot.TrustAnchors...), nil
}
