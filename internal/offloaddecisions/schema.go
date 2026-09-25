package offloaddecisions

import (
	"context"
	"database/sql"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "offloaddecisions" }

func (SchemaModule) Version() int { return 1 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS offload_decisions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			recorded_at INTEGER NOT NULL,
			node_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			trigger TEXT NOT NULL DEFAULT '',
			lane TEXT NOT NULL,
			owner_node_id TEXT NOT NULL DEFAULT '',
			owner_model_id TEXT NOT NULL DEFAULT '',
			helper_node_id TEXT NOT NULL DEFAULT '',
			helper_model_id TEXT NOT NULL DEFAULT '',
			outcome TEXT NOT NULL,
			reason TEXT NOT NULL DEFAULT '',
			pending_count INTEGER NOT NULL DEFAULT 0,
			backlog_count INTEGER NOT NULL DEFAULT 0,
			keep_ms REAL NOT NULL DEFAULT 0,
			switch_ms REAL NOT NULL DEFAULT 0,
			service_ms REAL NOT NULL DEFAULT 0,
			owner_job_ms REAL NOT NULL DEFAULT 0,
			service_source TEXT NOT NULL DEFAULT '',
			helper_idle_ms INTEGER NOT NULL DEFAULT 0,
			slots INTEGER NOT NULL DEFAULT 0,
			lent_out INTEGER NOT NULL DEFAULT 0,
			borrowed_ahead INTEGER NOT NULL DEFAULT 0,
			wait_ms INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS offload_decisions_recorded ON offload_decisions(recorded_at DESC)`,
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
