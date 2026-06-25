package analytics

import (
	"context"
	"database/sql"
)

func migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA synchronous = NORMAL`,
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS analytics_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			section TEXT NOT NULL,
			backend_mode TEXT NOT NULL,
			route TEXT NOT NULL,
			status_code INTEGER NOT NULL,
			success INTEGER NOT NULL,
			started_at INTEGER NOT NULL,
			finished_at INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			tokens_per_second REAL NOT NULL DEFAULT 0,
			image_count INTEGER NOT NULL DEFAULT 0,
			image_width INTEGER NOT NULL DEFAULT 0,
			image_height INTEGER NOT NULL DEFAULT 0,
			image_type TEXT NOT NULL DEFAULT '',
			audio_seconds REAL NOT NULL DEFAULT 0,
			audio_tokens INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS analytics_events_finished_at_idx ON analytics_events (finished_at)`,
		`CREATE INDEX IF NOT EXISTS analytics_events_node_idx ON analytics_events (node_id, finished_at)`,
		`CREATE INDEX IF NOT EXISTS analytics_events_model_idx ON analytics_events (model_id, finished_at)`,
		`CREATE INDEX IF NOT EXISTS analytics_events_section_idx ON analytics_events (section, finished_at)`,
		`CREATE INDEX IF NOT EXISTS analytics_events_common_idx ON analytics_events (node_id, model_id, section, finished_at)`,
		`CREATE TABLE IF NOT EXISTS analytics_rollups (
			period_kind TEXT NOT NULL,
			bucket_start INTEGER NOT NULL,
			node_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			section TEXT NOT NULL,
			backend_mode TEXT NOT NULL,
			route TEXT NOT NULL,
			request_count INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			duration_ms_total INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			tokens_per_second_sum REAL NOT NULL DEFAULT 0,
			tokens_per_second_count INTEGER NOT NULL DEFAULT 0,
			image_count INTEGER NOT NULL DEFAULT 0,
			audio_seconds REAL NOT NULL DEFAULT 0,
			audio_tokens INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (period_kind, bucket_start, node_id, model_id, section, backend_mode, route)
		)`,
		`CREATE INDEX IF NOT EXISTS analytics_rollups_period_idx ON analytics_rollups (period_kind, bucket_start)`,
		`CREATE INDEX IF NOT EXISTS analytics_rollups_common_idx ON analytics_rollups (period_kind, node_id, model_id, section, bucket_start)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	_, err := db.ExecContext(ctx, `PRAGMA user_version = 1`)
	return err
}
