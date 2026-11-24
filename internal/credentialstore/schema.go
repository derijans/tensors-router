package credentialstore

import (
	"context"
	"database/sql"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "credentials" }

func (SchemaModule) Version() int { return 1 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS credential_tokens (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			role TEXT NOT NULL CHECK (role IN ('inference', 'admin', 'metrics', 'cluster')),
			secret_hash BLOB NOT NULL UNIQUE,
			cluster_secret TEXT,
			source TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			CHECK ((role = 'cluster') = (cluster_secret IS NOT NULL))
		)`,
		`CREATE TABLE IF NOT EXISTS credential_certificates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL CHECK (kind IN ('identity', 'authority', 'trusted_authority', 'pinned_peer')),
			certificate_pem TEXT NOT NULL,
			private_key_pem TEXT,
			fingerprint TEXT NOT NULL UNIQUE,
			active INTEGER NOT NULL DEFAULT 0 CHECK (active IN (0, 1)),
			created_at INTEGER NOT NULL,
			CHECK (active = 0 OR kind = 'identity'),
			CHECK ((kind IN ('identity', 'authority')) = (private_key_pem IS NOT NULL))
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS credential_certificates_one_active_identity
			ON credential_certificates (active) WHERE active = 1`,
		`CREATE TABLE IF NOT EXISTS credential_revision (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			revision INTEGER NOT NULL
		)`,
		`INSERT OR IGNORE INTO credential_revision (id, revision) VALUES (1, 0)`,
		`CREATE TABLE IF NOT EXISTS credential_imports (
			source TEXT PRIMARY KEY,
			imported_at INTEGER NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (SchemaModule) ImportLegacy(context.Context, *sql.Tx, string) (int64, error) {
	return 0, nil
}
