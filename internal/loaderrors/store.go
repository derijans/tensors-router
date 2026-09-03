package loaderrors

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const defaultMaxOutputBytes = 64 * 1024

type StoreConfig struct {
	NodeID         string
	DatabasePath   string
	Retention      time.Duration
	MaxOutputBytes int
}

type Store struct {
	db             *sql.DB
	nodeID         string
	retention      time.Duration
	maxOutputBytes int

	pruneMu   sync.Mutex
	prunedAt  time.Time
	pruneEach time.Duration
}

func NewStore(config StoreConfig) (*Store, error) {
	path := strings.TrimSpace(config.DatabasePath)
	if path == "" {
		return nil, fmt.Errorf("load error database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	if err := migrate(context.Background(), database); err != nil {
		_ = database.Close()
		return nil, err
	}
	if err := secureDatabaseFiles(path); err != nil {
		_ = database.Close()
		return nil, err
	}
	nodeID := strings.TrimSpace(config.NodeID)
	if nodeID == "" {
		nodeID = "local"
	}
	maxOutput := config.MaxOutputBytes
	if maxOutput <= 0 {
		maxOutput = defaultMaxOutputBytes
	}
	return &Store{db: database, nodeID: nodeID, retention: config.Retention, maxOutputBytes: maxOutput, pruneEach: time.Minute}, nil
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

func (store *Store) Record(ctx context.Context, input RecordInput) error {
	if store == nil {
		return nil
	}
	message := strings.TrimSpace(redact(input.Message, input.Secrets))
	if message == "" {
		message = "(no message)"
	}
	output, truncated := clampOutput(redact(input.Output, input.Secrets), store.maxOutputBytes)
	if input.Truncated {
		truncated = true
	}
	severity := input.Severity
	if severity != SeverityWarning {
		severity = SeverityError
	}
	nodeID := strings.TrimSpace(input.NodeID)
	if nodeID == "" {
		nodeID = store.nodeID
	}
	now := time.Now().UTC().UnixMilli()
	print := fingerprint(input, message)
	_, err := store.db.ExecContext(ctx, `INSERT INTO load_errors (
			id, fingerprint, first_seen_at, last_seen_at, occurrences,
			node_id, model_id, config_name, backend, backend_mode,
			phase, severity, source, message, exit_error, output, truncated
		) VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(fingerprint) DO UPDATE SET
			last_seen_at = excluded.last_seen_at,
			occurrences = load_errors.occurrences + 1,
			node_id = excluded.node_id,
			model_id = excluded.model_id,
			config_name = excluded.config_name,
			backend = excluded.backend,
			backend_mode = excluded.backend_mode,
			severity = excluded.severity,
			source = excluded.source,
			message = excluded.message,
			exit_error = excluded.exit_error,
			output = excluded.output,
			truncated = excluded.truncated`,
		uuid.NewString(), print, now, now,
		nodeID, strings.TrimSpace(input.ModelID), strings.TrimSpace(input.ConfigName), strings.TrimSpace(input.Backend), strings.TrimSpace(input.BackendMode),
		string(phaseOrOther(input.Phase)), string(severity), strings.TrimSpace(input.Source), message, strings.TrimSpace(redact(input.ExitError, input.Secrets)), output, boolValue(truncated),
	)
	if err != nil {
		return err
	}
	store.pruneExpired(ctx)
	return nil
}

func (store *Store) List(ctx context.Context, filter ListFilter) (ListResult, error) {
	if store == nil {
		return ListResult{}, nil
	}
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	conditions := make([]string, 0, 3)
	arguments := make([]any, 0, 4)
	if node := strings.TrimSpace(filter.NodeID); node != "" {
		conditions = append(conditions, "node_id = ?")
		arguments = append(arguments, node)
	}
	if filter.Phase != "" {
		conditions = append(conditions, "phase = ?")
		arguments = append(arguments, string(filter.Phase))
	}
	if filter.Severity != "" {
		conditions = append(conditions, "severity = ?")
		arguments = append(arguments, string(filter.Severity))
	}
	query := `SELECT id, fingerprint, first_seen_at, last_seen_at, occurrences, node_id, model_id, config_name, backend, backend_mode, phase, severity, source, message, exit_error, output, truncated FROM load_errors`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY last_seen_at DESC, id DESC LIMIT ?"
	arguments = append(arguments, limit)
	rows, err := store.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return ListResult{}, err
	}
	defer rows.Close()
	result := ListResult{Records: []Record{}}
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return ListResult{}, scanErr
		}
		result.Records = append(result.Records, record)
	}
	return result, rows.Err()
}

func (store *Store) Get(ctx context.Context, id string) (Record, bool, error) {
	if store == nil {
		return Record{}, false, nil
	}
	row := store.db.QueryRowContext(ctx, `SELECT id, fingerprint, first_seen_at, last_seen_at, occurrences, node_id, model_id, config_name, backend, backend_mode, phase, severity, source, message, exit_error, output, truncated FROM load_errors WHERE id = ?`, id)
	record, err := scanRecord(row)
	if err == sql.ErrNoRows {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}

func (store *Store) pruneExpired(ctx context.Context) {
	if store.retention <= 0 {
		return
	}
	store.pruneMu.Lock()
	if !store.prunedAt.IsZero() && time.Since(store.prunedAt) < store.pruneEach {
		store.pruneMu.Unlock()
		return
	}
	store.prunedAt = time.Now()
	store.pruneMu.Unlock()
	cutoff := time.Now().UTC().Add(-store.retention).UnixMilli()
	_, _ = store.db.ExecContext(ctx, `DELETE FROM load_errors WHERE last_seen_at < ?`, cutoff)
}

func scanRecord(scanner interface{ Scan(...any) error }) (Record, error) {
	var record Record
	var phase, severity string
	var first, last int64
	var truncated int
	err := scanner.Scan(&record.ID, &record.Fingerprint, &first, &last, &record.Occurrences,
		&record.NodeID, &record.ModelID, &record.ConfigName, &record.Backend, &record.BackendMode,
		&phase, &severity, &record.Source, &record.Message, &record.ExitError, &record.Output, &truncated)
	if err != nil {
		return Record{}, err
	}
	record.Phase = Phase(phase)
	record.Severity = Severity(severity)
	record.FirstSeenAt = time.UnixMilli(first).UTC()
	record.LastSeenAt = time.UnixMilli(last).UTC()
	record.Truncated = truncated != 0
	return record, nil
}

func phaseOrOther(phase Phase) Phase {
	if strings.TrimSpace(string(phase)) == "" {
		return "other"
	}
	return phase
}

func boolValue(value bool) int {
	if value {
		return 1
	}
	return 0
}

func secureDatabaseFiles(path string) error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		candidate := path + suffix
		if _, err := os.Stat(candidate); err == nil {
			if err := os.Chmod(candidate, 0o600); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
