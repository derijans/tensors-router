package modelstate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const databaseFilename = "model-state.sqlite"

type Store struct {
	database *sql.DB
}

func NewStore(directory string) (*Store, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, fmt.Errorf("model state store dir is required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", filepath.Join(directory, databaseFilename))
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	store := &Store{database: database}
	if err := store.initialize(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

func (store *Store) initialize() error {
	_, err := store.database.Exec("PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; CREATE TABLE IF NOT EXISTS disabled_models (local_id TEXT PRIMARY KEY NOT NULL, disabled_at INTEGER NOT NULL DEFAULT (unixepoch()));")
	return err
}

func (store *Store) Disabled(ctx context.Context, localID string) (bool, error) {
	localID = strings.TrimSpace(localID)
	if localID == "" {
		return false, fmt.Errorf("local_id is required")
	}
	var found int
	err := store.database.QueryRowContext(ctx, "SELECT 1 FROM disabled_models WHERE local_id = ?", localID).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (store *Store) DisabledIDs(ctx context.Context) (map[string]struct{}, error) {
	rows, err := store.database.QueryContext(ctx, "SELECT local_id FROM disabled_models")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	disabled := map[string]struct{}{}
	for rows.Next() {
		var localID string
		if err := rows.Scan(&localID); err != nil {
			return nil, err
		}
		disabled[localID] = struct{}{}
	}
	return disabled, rows.Err()
}

func (store *Store) SetEnabled(ctx context.Context, localID string, enabled bool) error {
	localID = strings.TrimSpace(localID)
	if localID == "" {
		return fmt.Errorf("local_id is required")
	}
	if enabled {
		_, err := store.database.ExecContext(ctx, "DELETE FROM disabled_models WHERE local_id = ?", localID)
		return err
	}
	_, err := store.database.ExecContext(ctx, "INSERT INTO disabled_models(local_id) VALUES (?) ON CONFLICT(local_id) DO NOTHING", localID)
	return err
}

func (store *Store) Close() error {
	if store == nil || store.database == nil {
		return nil
	}
	return store.database.Close()
}
