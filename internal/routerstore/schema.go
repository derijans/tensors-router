package routerstore

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Module is one subsystem's share of the router database: the tables it owns,
// the version those tables are at, and the way rows are lifted out of the
// standalone database the subsystem used before the databases were joined.
type Module interface {
	Name() string
	Version() int
	Migrate(ctx context.Context, db *sql.DB) error
	ImportLegacy(ctx context.Context, tx *sql.Tx, legacySchema string) (int64, error)
}

const fileFormatVersion = 6

func applyFileFormatVersion(ctx context.Context, db *sql.DB) error {
	var current int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return err
	}
	if current >= fileFormatVersion {
		return nil
	}
	_, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, fileFormatVersion))
	return err
}

func createRegistryTables(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS routerstore_schema_versions (
			module TEXT PRIMARY KEY,
			version INTEGER NOT NULL,
			applied_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS routerstore_legacy_imports (
			module TEXT NOT NULL,
			source TEXT NOT NULL,
			rows_copied INTEGER NOT NULL,
			imported_at INTEGER NOT NULL,
			PRIMARY KEY (module, source)
		)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func migrateModules(ctx context.Context, db *sql.DB, modules []Module) error {
	for _, module := range modules {
		if err := module.Migrate(ctx, db); err != nil {
			return fmt.Errorf("migrate %s schema: %w", module.Name(), err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO routerstore_schema_versions (module, version, applied_at)
			VALUES (?, ?, ?)
			ON CONFLICT(module) DO UPDATE SET version = excluded.version, applied_at = excluded.applied_at`,
			module.Name(), module.Version(), time.Now().UTC().UnixMilli()); err != nil {
			return err
		}
	}
	return nil
}
