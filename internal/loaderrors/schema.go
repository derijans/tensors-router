package loaderrors

import (
	"context"
	"database/sql"
	"strings"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "loaderrors" }

func (SchemaModule) Version() int { return 1 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
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
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (SchemaModule) ImportLegacy(ctx context.Context, tx *sql.Tx, legacySchema string) (int64, error) {
	return routerstore.CopyRows(ctx, tx, routerstore.CopySpec{
		LegacySchema: legacySchema,
		LegacyTable:  "load_errors",
		Table:        "load_errors",
		Columns:      importColumns(),
		Conflict:     mergeByFingerprint,
	})
}

func importColumns() []string {
	return []string{
		"id", "fingerprint", "first_seen_at", "last_seen_at", "occurrences",
		"node_id", "model_id", "config_name", "backend", "backend_mode",
		"phase", "severity", "source", "message", "exit_error", "output", "truncated",
	}
}

// A fingerprint identifies the same failure seen twice, so the two rows are one
// history: the window widens, the occurrence counts add, and the description is
// taken from whichever sighting is newer.
func mergeByFingerprint(columns []string) string {
	present := make(map[string]bool, len(columns))
	for _, column := range columns {
		present[column] = true
	}
	assignments := make([]string, 0, len(columns))
	if present["first_seen_at"] {
		assignments = append(assignments, "first_seen_at = MIN(load_errors.first_seen_at, excluded.first_seen_at)")
	}
	if present["last_seen_at"] {
		assignments = append(assignments, "last_seen_at = MAX(load_errors.last_seen_at, excluded.last_seen_at)")
	}
	if present["occurrences"] {
		assignments = append(assignments, "occurrences = load_errors.occurrences + excluded.occurrences")
	}
	for _, column := range descriptiveColumns() {
		if !present[column] || !present["last_seen_at"] {
			continue
		}
		assignments = append(assignments, column+" = CASE WHEN excluded.last_seen_at >= load_errors.last_seen_at THEN excluded."+column+" ELSE load_errors."+column+" END")
	}
	if len(assignments) == 0 {
		return "ON CONFLICT(fingerprint) DO NOTHING"
	}
	return "ON CONFLICT(fingerprint) DO UPDATE SET " + strings.Join(assignments, ", ")
}

func descriptiveColumns() []string {
	return []string{
		"node_id", "model_id", "config_name", "backend", "backend_mode",
		"phase", "severity", "source", "message", "exit_error", "output", "truncated",
	}
}
