package loaderrors

import (
	"context"
	"database/sql"
)

func migrate(ctx context.Context, database *sql.DB) error {
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=NORMAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS load_errors (
			id TEXT PRIMARY KEY,
			fingerprint TEXT NOT NULL UNIQUE,
			first_seen_at INTEGER NOT NULL,
			last_seen_at INTEGER NOT NULL,
			occurrences INTEGER NOT NULL DEFAULT 1,
			node_id TEXT NOT NULL DEFAULT '',
			model_id TEXT NOT NULL DEFAULT '',
			config_name TEXT NOT NULL DEFAULT '',
			backend TEXT NOT NULL DEFAULT '',
			backend_mode TEXT NOT NULL DEFAULT '',
			phase TEXT NOT NULL,
			severity TEXT NOT NULL DEFAULT 'error',
			source TEXT NOT NULL DEFAULT '',
			message TEXT NOT NULL,
			exit_error TEXT NOT NULL DEFAULT '',
			output TEXT NOT NULL DEFAULT '',
			truncated INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS load_errors_last_seen ON load_errors(last_seen_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS load_errors_phase_last_seen ON load_errors(phase, last_seen_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS load_errors_node_last_seen ON load_errors(node_id, last_seen_at DESC, id DESC)`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
