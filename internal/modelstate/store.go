package modelstate

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	_, err := store.database.Exec(`
		PRAGMA busy_timeout = 5000;
		PRAGMA journal_mode = WAL;
		CREATE TABLE IF NOT EXISTS disabled_models (
			local_id    TEXT PRIMARY KEY NOT NULL,
			disabled_at INTEGER NOT NULL DEFAULT (unixepoch())
		);
		CREATE TABLE IF NOT EXISTS separate_runtime (
			local_id        TEXT PRIMARY KEY NOT NULL,
			run_separate    INTEGER NOT NULL DEFAULT 0,
			unload_triggers TEXT    NOT NULL DEFAULT '[]',
			updated_at      INTEGER NOT NULL DEFAULT (unixepoch())
		);
	`)
	return err
}

type SeparateRuntimeSettings struct {
	RunSeparate    bool
	UnloadTriggers []string
}

func (store *Store) SeparateRuntime(ctx context.Context, localID string) (SeparateRuntimeSettings, bool, error) {
	localID = strings.TrimSpace(localID)
	if localID == "" {
		return SeparateRuntimeSettings{}, false, fmt.Errorf("local_id is required")
	}
	var runSeparate int
	var triggersJSON string
	err := store.database.QueryRowContext(ctx,
		"SELECT run_separate, unload_triggers FROM separate_runtime WHERE local_id = ?", localID,
	).Scan(&runSeparate, &triggersJSON)
	if err == sql.ErrNoRows {
		return SeparateRuntimeSettings{}, false, nil
	}
	if err != nil {
		return SeparateRuntimeSettings{}, false, err
	}
	triggers, err := decodeTriggers(triggersJSON)
	if err != nil {
		return SeparateRuntimeSettings{}, false, err
	}
	return SeparateRuntimeSettings{RunSeparate: runSeparate != 0, UnloadTriggers: triggers}, true, nil
}

func (store *Store) AllSeparateRuntimes(ctx context.Context) (map[string]SeparateRuntimeSettings, error) {
	rows, err := store.database.QueryContext(ctx, "SELECT local_id, run_separate, unload_triggers FROM separate_runtime")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	overrides := map[string]SeparateRuntimeSettings{}
	for rows.Next() {
		var localID, triggersJSON string
		var runSeparate int
		if err := rows.Scan(&localID, &runSeparate, &triggersJSON); err != nil {
			return nil, err
		}
		triggers, err := decodeTriggers(triggersJSON)
		if err != nil {
			return nil, err
		}
		overrides[localID] = SeparateRuntimeSettings{RunSeparate: runSeparate != 0, UnloadTriggers: triggers}
	}
	return overrides, rows.Err()
}

func (store *Store) SetSeparateRuntime(ctx context.Context, localID string, settings SeparateRuntimeSettings) error {
	localID = strings.TrimSpace(localID)
	if localID == "" {
		return fmt.Errorf("local_id is required")
	}
	triggersJSON, err := encodeTriggers(settings.UnloadTriggers)
	if err != nil {
		return err
	}
	runSeparate := 0
	if settings.RunSeparate {
		runSeparate = 1
	}
	_, err = store.database.ExecContext(ctx, `
		INSERT INTO separate_runtime(local_id, run_separate, unload_triggers, updated_at)
		VALUES (?, ?, ?, unixepoch())
		ON CONFLICT(local_id) DO UPDATE SET
			run_separate    = excluded.run_separate,
			unload_triggers = excluded.unload_triggers,
			updated_at      = excluded.updated_at
	`, localID, runSeparate, triggersJSON)
	return err
}

func (store *Store) ClearSeparateRuntime(ctx context.Context, localID string) error {
	localID = strings.TrimSpace(localID)
	if localID == "" {
		return fmt.Errorf("local_id is required")
	}
	_, err := store.database.ExecContext(ctx, "DELETE FROM separate_runtime WHERE local_id = ?", localID)
	return err
}

func decodeTriggers(encoded string) ([]string, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	var triggers []string
	if err := json.Unmarshal([]byte(encoded), &triggers); err != nil {
		return nil, err
	}
	return triggers, nil
}

func encodeTriggers(triggers []string) (string, error) {
	cleaned := make([]string, 0, len(triggers))
	seen := map[string]struct{}{}
	for _, trigger := range triggers {
		trigger = strings.TrimSpace(trigger)
		if trigger == "" {
			continue
		}
		if _, ok := seen[trigger]; ok {
			continue
		}
		seen[trigger] = struct{}{}
		cleaned = append(cleaned, trigger)
	}
	sort.Strings(cleaned)
	encoded, err := json.Marshal(cleaned)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
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
