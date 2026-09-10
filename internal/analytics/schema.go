package analytics

import (
	"context"
	"database/sql"
	"fmt"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "analytics" }

func (SchemaModule) Version() int { return 6 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS analytics_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			node_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			section TEXT NOT NULL,
			backend_mode TEXT NOT NULL,
			event_type TEXT NOT NULL DEFAULT 'request',
			route TEXT NOT NULL,
			config_filename TEXT NOT NULL DEFAULT '',
			status_code INTEGER NOT NULL,
			success INTEGER NOT NULL,
			started_at INTEGER NOT NULL,
			finished_at INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			request_bytes INTEGER NOT NULL DEFAULT 0,
			response_bytes INTEGER NOT NULL DEFAULT 0,
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			tokens_per_second REAL NOT NULL DEFAULT 0,
			image_count INTEGER NOT NULL DEFAULT 0,
			image_width INTEGER NOT NULL DEFAULT 0,
			image_height INTEGER NOT NULL DEFAULT 0,
			image_steps INTEGER NOT NULL DEFAULT 0,
			image_type TEXT NOT NULL DEFAULT '',
			audio_seconds REAL NOT NULL DEFAULT 0,
			audio_tokens INTEGER NOT NULL DEFAULT 0,
			audio_language TEXT NOT NULL DEFAULT '',
			audio_task TEXT NOT NULL DEFAULT '',
			load_vram_before_mb INTEGER NOT NULL DEFAULT 0,
			load_vram_after_mb INTEGER NOT NULL DEFAULT 0,
			load_vram_delta_mb INTEGER NOT NULL DEFAULT 0,
			work_vram_start_mb INTEGER NOT NULL DEFAULT 0,
			work_vram_max_mb INTEGER NOT NULL DEFAULT 0,
			work_vram_end_mb INTEGER NOT NULL DEFAULT 0,
			model_vram_estimate_mb INTEGER NOT NULL DEFAULT 0,
			vram_total_mb INTEGER NOT NULL DEFAULT 0,
			vram_peak_percent REAL NOT NULL DEFAULT 0,
			ttft_ms INTEGER NOT NULL DEFAULT 0,
			decode_ms INTEGER NOT NULL DEFAULT 0,
			max_gap_ms INTEGER NOT NULL DEFAULT 0,
			finish_reason TEXT NOT NULL DEFAULT '',
			aborted INTEGER NOT NULL DEFAULT 0
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
			load_count INTEGER NOT NULL DEFAULT 0,
			load_duration_ms_total INTEGER NOT NULL DEFAULT 0,
			vram_peak_mb INTEGER NOT NULL DEFAULT 0,
			vram_peak_percent REAL NOT NULL DEFAULT 0,
			vram_total_mb INTEGER NOT NULL DEFAULT 0,
			model_vram_estimate_mb INTEGER NOT NULL DEFAULT 0,
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
	for _, column := range migrationColumns() {
		if err := addColumnIfMissing(ctx, db, column.table, column.name, column.definition); err != nil {
			return err
		}
	}
	return nil
}

func (SchemaModule) ImportLegacy(ctx context.Context, tx *sql.Tx, legacySchema string) (int64, error) {
	events, err := routerstore.CopyRows(ctx, tx, routerstore.CopySpec{
		LegacySchema: legacySchema,
		LegacyTable:  "analytics_events",
		Table:        "analytics_events",
		Columns:      eventColumns(),
	})
	if err != nil {
		return 0, err
	}
	rollups, err := routerstore.CopyRows(ctx, tx, routerstore.CopySpec{
		LegacySchema: legacySchema,
		LegacyTable:  "analytics_rollups",
		Table:        "analytics_rollups",
		Columns:      rollupColumns(),
		Conflict:     rollupMergeClause,
	})
	if err != nil {
		return 0, err
	}
	return events + rollups, nil
}

// The surrogate key is left behind so the merged table numbers its own rows;
// nothing refers to an event by id.
func eventColumns() []string {
	return []string{
		"node_id", "model_id", "section", "backend_mode", "event_type", "route", "config_filename",
		"status_code", "success", "started_at", "finished_at", "duration_ms", "request_bytes",
		"response_bytes", "input_tokens", "output_tokens", "total_tokens", "tokens_per_second",
		"image_count", "image_width", "image_height", "image_steps", "image_type", "audio_seconds",
		"audio_tokens", "audio_language", "audio_task", "load_vram_before_mb", "load_vram_after_mb",
		"load_vram_delta_mb", "work_vram_start_mb", "work_vram_max_mb", "work_vram_end_mb",
		"model_vram_estimate_mb", "vram_total_mb", "vram_peak_percent",
	}
}

type migrationColumn struct {
	table      string
	name       string
	definition string
}

func migrationColumns() []migrationColumn {
	return []migrationColumn{
		{"analytics_events", "event_type", "TEXT NOT NULL DEFAULT 'request'"},
		{"analytics_events", "config_filename", "TEXT NOT NULL DEFAULT ''"},
		{"analytics_events", "request_bytes", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "response_bytes", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "image_steps", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "audio_language", "TEXT NOT NULL DEFAULT ''"},
		{"analytics_events", "audio_task", "TEXT NOT NULL DEFAULT ''"},
		{"analytics_events", "load_vram_before_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "load_vram_after_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "load_vram_delta_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "work_vram_start_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "work_vram_max_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "work_vram_end_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "model_vram_estimate_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "vram_total_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "vram_peak_percent", "REAL NOT NULL DEFAULT 0"},
		{"analytics_events", "ttft_ms", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "decode_ms", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "max_gap_ms", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_events", "finish_reason", "TEXT NOT NULL DEFAULT ''"},
		{"analytics_events", "aborted", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_rollups", "load_count", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_rollups", "load_duration_ms_total", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_rollups", "vram_peak_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_rollups", "vram_peak_percent", "REAL NOT NULL DEFAULT 0"},
		{"analytics_rollups", "vram_total_mb", "INTEGER NOT NULL DEFAULT 0"},
		{"analytics_rollups", "model_vram_estimate_mb", "INTEGER NOT NULL DEFAULT 0"},
	}
}

func addColumnIfMissing(ctx context.Context, db *sql.DB, table string, name string, definition string) error {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var columnName string
		var columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&id, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if columnName == name {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, definition))
	return err
}
