package loadcapture

import (
	"context"
	"database/sql"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "loadcapture" }

func (SchemaModule) Version() int { return 1 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS load_capture_snapshots (sha256 TEXT PRIMARY KEY, payload BLOB NOT NULL, created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS load_capture_snapshot_assets (snapshot_sha256 TEXT NOT NULL, role TEXT NOT NULL, position INTEGER NOT NULL, sha256 TEXT NOT NULL, PRIMARY KEY (snapshot_sha256, role, position), FOREIGN KEY (snapshot_sha256) REFERENCES load_capture_snapshots(sha256) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS load_capture_attempts (id TEXT PRIMARY KEY, node_id TEXT NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, backend_mode TEXT NOT NULL, runtime TEXT NOT NULL, lane TEXT NOT NULL, snapshot_sha256 TEXT NOT NULL, physical_attempt_id TEXT, started_at INTEGER NOT NULL, finished_at INTEGER NOT NULL DEFAULT 0, duration_ms INTEGER NOT NULL DEFAULT 0, failure_class TEXT NOT NULL DEFAULT '', failure_message TEXT NOT NULL DEFAULT '', captured_bytes INTEGER NOT NULL DEFAULT 0, truncated INTEGER NOT NULL DEFAULT 0, FOREIGN KEY (snapshot_sha256) REFERENCES load_capture_snapshots(sha256), FOREIGN KEY (physical_attempt_id) REFERENCES load_capture_attempts(id))`,
		`CREATE TABLE IF NOT EXISTS load_capture_output_chunks (physical_attempt_id TEXT NOT NULL, sequence INTEGER NOT NULL, stream TEXT NOT NULL, offset_ns INTEGER NOT NULL, payload BLOB NOT NULL, PRIMARY KEY (physical_attempt_id, sequence), FOREIGN KEY (physical_attempt_id) REFERENCES load_capture_attempts(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS load_capture_attempts_node_started ON load_capture_attempts(node_id, started_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS load_capture_attempts_status_started ON load_capture_attempts(status, started_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS load_capture_attempts_backend_started ON load_capture_attempts(backend_mode, started_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS load_capture_attempts_snapshot_started ON load_capture_attempts(snapshot_sha256, started_at DESC, id DESC)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (SchemaModule) ImportLegacy(ctx context.Context, tx *sql.Tx, legacySchema string) (int64, error) {
	// Snapshots and their assets are content addressed, so a hash already held
	// carries the same bytes and ignoring the duplicate loses nothing.
	specs := []routerstore.CopySpec{
		{
			LegacyTable: "snapshots",
			Table:       "load_capture_snapshots",
			Columns:     []string{"sha256", "payload", "created_at"},
			Conflict:    func([]string) string { return `ON CONFLICT(sha256) DO NOTHING` },
		},
		{
			LegacyTable: "snapshot_assets",
			Table:       "load_capture_snapshot_assets",
			Columns:     []string{"snapshot_sha256", "role", "position", "sha256"},
			Conflict: func([]string) string {
				return `ON CONFLICT(snapshot_sha256, role, position) DO NOTHING`
			},
		},
		{
			LegacyTable: "attempts",
			Table:       "load_capture_attempts",
			Columns: []string{
				"id", "node_id", "kind", "status", "backend_mode", "runtime", "lane",
				"snapshot_sha256", "physical_attempt_id", "started_at", "finished_at",
				"duration_ms", "failure_class", "failure_message", "captured_bytes", "truncated",
			},
		},
		{
			LegacyTable: "output_chunks",
			Table:       "load_capture_output_chunks",
			Columns:     []string{"physical_attempt_id", "sequence", "stream", "offset_ns", "payload"},
		},
	}
	var copied int64
	for _, spec := range specs {
		spec.LegacySchema = legacySchema
		rows, err := routerstore.CopyRows(ctx, tx, spec)
		if err != nil {
			return 0, err
		}
		copied += rows
	}
	return copied, nil
}
