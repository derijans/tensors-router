package memorysamples

import (
	"context"
	"database/sql"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "memorysamples" }

func (SchemaModule) Version() int { return 1 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS memory_fit_samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			combo_key TEXT NOT NULL,
			placement_key TEXT NOT NULL,
			node_id TEXT NOT NULL,
			architecture TEXT NOT NULL DEFAULT '',
			context_size INTEGER NOT NULL,
			vram_mb INTEGER NOT NULL,
			ram_mb INTEGER NOT NULL,
			predicted_gpu_mb REAL NOT NULL DEFAULT 0,
			recorded_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS memory_fit_samples_combo ON memory_fit_samples(combo_key, id DESC)`,
		`CREATE INDEX IF NOT EXISTS memory_fit_samples_placement ON memory_fit_samples(placement_key, id DESC)`,
		`CREATE INDEX IF NOT EXISTS memory_fit_samples_architecture ON memory_fit_samples(node_id, architecture, id DESC)`,
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
