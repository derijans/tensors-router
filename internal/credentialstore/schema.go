package credentialstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "credentials" }

func (SchemaModule) Version() int { return 3 }

const (
	certificatesTable             = "credential_certificates"
	rebuiltCertificatesTable      = "credential_certificates_rebuild"
	certificateKindsCheck         = "kind IN ('identity', 'authority', 'trusted_authority', 'pinned_peer', 'api_certificate')"
	oneActivePerKindIndex         = `CREATE UNIQUE INDEX IF NOT EXISTS credential_certificates_one_active_per_kind ON credential_certificates (kind) WHERE active = 1`
	dropLegacyActiveIdentityIndex = `DROP INDEX IF EXISTS credential_certificates_one_active_identity`
	sourceColumnDefinition        = "source TEXT NOT NULL"
	pendingKeysTableDefinition    = `CREATE TABLE IF NOT EXISTS credential_pending_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL CHECK (kind IN ('identity', 'api_certificate')),
			request_pem TEXT NOT NULL,
			private_key_pem TEXT NOT NULL,
			public_key_fingerprint TEXT NOT NULL UNIQUE,
			created_at INTEGER NOT NULL
		)`
)

func certificatesTableDefinition(table string) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			kind TEXT NOT NULL CHECK (%s),
			certificate_pem TEXT NOT NULL,
			private_key_pem TEXT,
			fingerprint TEXT NOT NULL UNIQUE,
			active INTEGER NOT NULL DEFAULT 0 CHECK (active IN (0, 1)),
			created_at INTEGER NOT NULL,
			source TEXT NOT NULL DEFAULT 'supplied',
			CHECK (active = 0 OR kind IN ('identity', 'api_certificate')),
			CHECK ((kind IN ('identity', 'authority', 'api_certificate')) = (private_key_pem IS NOT NULL))
		)`, table, certificateKindsCheck)
}

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
		certificatesTableDefinition(certificatesTable),
		`CREATE TABLE IF NOT EXISTS credential_revision (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			revision INTEGER NOT NULL
		)`,
		`INSERT OR IGNORE INTO credential_revision (id, revision) VALUES (1, 0)`,
		`CREATE TABLE IF NOT EXISTS credential_imports (
			source TEXT PRIMARY KEY,
			imported_at INTEGER NOT NULL
		)`,
		pendingKeysTableDefinition,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := rebuildCertificatesTableIfOutdated(ctx, db); err != nil {
		return err
	}
	for _, statement := range []string{dropLegacyActiveIdentityIndex, oneActivePerKindIndex} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func rebuildCertificatesTableIfOutdated(ctx context.Context, db *sql.DB) error {
	var definition string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, certificatesTable).Scan(&definition); err != nil {
		return err
	}
	if strings.Contains(definition, sourceColumnDefinition) {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	statements := []string{
		certificatesTableDefinition(rebuiltCertificatesTable),
		`INSERT INTO ` + rebuiltCertificatesTable + ` (id, name, kind, certificate_pem, private_key_pem, fingerprint, active, created_at, source)
			SELECT id, name, kind, certificate_pem, private_key_pem, fingerprint, active, created_at,
				CASE WHEN kind = 'api_certificate' AND name LIKE 'migrated-%' THEN 'migrated' ELSE 'supplied' END
			FROM ` + certificatesTable,
		`DROP TABLE ` + certificatesTable,
		`ALTER TABLE ` + rebuiltCertificatesTable + ` RENAME TO ` + certificatesTable,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild %s: %w", certificatesTable, err)
		}
	}
	return tx.Commit()
}

func (SchemaModule) ImportLegacy(context.Context, *sql.Tx, string) (int64, error) {
	return 0, nil
}
