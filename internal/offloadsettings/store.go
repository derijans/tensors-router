package offloadsettings

import (
	"context"
	"database/sql"
	"time"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "offloadsettings" }

func (SchemaModule) Version() int { return 1 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS offload_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at INTEGER NOT NULL
	)`)
	return err
}

func (SchemaModule) ImportLegacy(context.Context, *sql.Tx, string) (int64, error) {
	return 0, nil
}

type Store struct {
	writer *sql.DB
	reader *sql.DB
}

func NewStore(writer *sql.DB, reader *sql.DB) *Store {
	return &Store{writer: writer, reader: reader}
}

func (store *Store) Overrides(ctx context.Context) (Values, error) {
	rows, err := store.reader.QueryContext(ctx, `SELECT key, value FROM offload_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	overrides := Values{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		overrides[key] = value
	}
	return overrides, rows.Err()
}

func (store *Store) Set(ctx context.Context, values Values) error {
	normalized := make(Values, len(values))
	for key, value := range values {
		canonical, err := Normalize(key, value)
		if err != nil {
			return err
		}
		normalized[key] = canonical
	}
	transaction, err := store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	updatedAt := time.Now().UTC().UnixMilli()
	for key, value := range normalized {
		if _, err := transaction.ExecContext(ctx, `INSERT INTO offload_settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, updatedAt); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (store *Store) Clear(ctx context.Context, key string) error {
	if _, err := fieldFor(key); err != nil {
		return err
	}
	_, err := store.writer.ExecContext(ctx, `DELETE FROM offload_settings WHERE key = ?`, key)
	return err
}

func (store *Store) ClearAll(ctx context.Context) error {
	_, err := store.writer.ExecContext(ctx, `DELETE FROM offload_settings`)
	return err
}
