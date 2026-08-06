package loadcapture

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type Kind string
type Status string

const (
	KindPhysical      Kind   = "physical"
	KindReuse         Kind   = "reuse"
	StatusLoading     Status = "loading"
	StatusSucceeded   Status = "succeeded"
	StatusFailed      Status = "failed"
	StatusInterrupted Status = "interrupted"
	StatusReused      Status = "reused"
)

type Attempt struct {
	ID                string    `json:"id"`
	NodeID            string    `json:"node_id"`
	Kind              Kind      `json:"kind"`
	Status            Status    `json:"status"`
	BackendMode       string    `json:"backend_mode"`
	Runtime           string    `json:"runtime"`
	Lane              string    `json:"lane"`
	SnapshotSHA256    string    `json:"snapshot_sha256"`
	PhysicalAttemptID string    `json:"physical_attempt_id,omitempty"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at,omitempty"`
	DurationMS        int64     `json:"duration_ms"`
	FailureClass      string    `json:"failure_class,omitempty"`
	FailureMessage    string    `json:"failure_message,omitempty"`
	CapturedBytes     int64     `json:"captured_bytes"`
	Truncated         bool      `json:"truncated"`
	ModelHashes       []string  `json:"model_hashes,omitempty"`
}

type Detail struct {
	Attempt  Attempt      `json:"attempt"`
	Snapshot jsonSnapshot `json:"snapshot"`
	Assets   []Asset      `json:"assets"`
}

type jsonSnapshot struct {
	SHA256 string `json:"sha256"`
	JSON   []byte `json:"json"`
}

type OutputPage struct {
	Chunks       []Chunk `json:"chunks"`
	NextSequence int64   `json:"next_sequence,omitempty"`
}

type StoreConfig struct {
	NodeID       string
	DatabasePath string
	Logger       *log.Logger
}

type Store struct {
	db     *sql.DB
	nodeID string
	logger *log.Logger
}

func NewStore(config StoreConfig) (*Store, error) {
	path := strings.TrimSpace(config.DatabasePath)
	if path == "" {
		return nil, fmt.Errorf("load capture database path is required")
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(parent, 0o700); err != nil {
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
	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}
	return &Store{db: database, nodeID: nodeID, logger: logger}, nil
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	return store.db.Close()
}

func (store *Store) BeginPhysical(ctx context.Context, snapshot Snapshot, backendMode string, runtime string, lane string) (Attempt, error) {
	if store == nil {
		return Attempt{}, nil
	}
	now := time.Now().UTC()
	attempt := Attempt{ID: uuid.NewString(), NodeID: store.nodeID, Kind: KindPhysical, Status: StatusLoading, BackendMode: strings.TrimSpace(backendMode), Runtime: strings.TrimSpace(runtime), Lane: strings.TrimSpace(lane), SnapshotSHA256: snapshot.SHA256, StartedAt: now}
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Attempt{}, err
	}
	if err = insertSnapshot(ctx, transaction, snapshot, now); err == nil {
		_, err = transaction.ExecContext(ctx, `INSERT INTO attempts (id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, physical_attempt_id, started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, 0, 0, '', '', 0, 0)`, attempt.ID, attempt.NodeID, attempt.Kind, attempt.Status, attempt.BackendMode, attempt.Runtime, attempt.Lane, attempt.SnapshotSHA256, attempt.StartedAt.UnixMilli())
	}
	if err != nil {
		_ = transaction.Rollback()
		return Attempt{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Attempt{}, err
	}
	return attempt, nil
}

func (store *Store) CompletePhysical(ctx context.Context, attempt Attempt, loadErr error, capture Capture, redactions map[string]string) error {
	if store == nil || attempt.ID == "" {
		return nil
	}
	finished := time.Now().UTC()
	status := StatusSucceeded
	if loadErr != nil {
		status = StatusFailed
	}
	redactions = captureRedactions(redactions, capture.Secrets)
	failureClass, failureMessage := failureFields(loadErr, redactions)
	chunks := sanitizeChunks(capture.Chunks, redactions)
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = transaction.ExecContext(ctx, `UPDATE attempts SET status = ?, finished_at = ?, duration_ms = ?, failure_class = ?, failure_message = ?, captured_bytes = ?, truncated = ? WHERE id = ? AND status = ?`, status, finished.UnixMilli(), finished.Sub(attempt.StartedAt).Milliseconds(), failureClass, failureMessage, capture.CapturedBytes, boolValue(capture.Truncated), attempt.ID, StatusLoading); err == nil {
		statement, prepareErr := transaction.PrepareContext(ctx, `INSERT INTO output_chunks (physical_attempt_id, sequence, stream, offset_ns, payload) VALUES (?, ?, ?, ?, ?)`)
		if prepareErr != nil {
			err = prepareErr
		} else {
			for _, chunk := range chunks {
				if _, err = statement.ExecContext(ctx, attempt.ID, chunk.Sequence, chunk.Stream, chunk.Offset.Nanoseconds(), chunk.Payload); err != nil {
					break
				}
			}
			closeErr := statement.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		_ = transaction.Rollback()
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	return secureDatabaseFiles(store.databasePath())
}

func (store *Store) RecordReuse(ctx context.Context, physicalAttemptID string) (Attempt, error) {
	if store == nil || strings.TrimSpace(physicalAttemptID) == "" {
		return Attempt{}, nil
	}
	var source Attempt
	err := store.db.QueryRowContext(ctx, `SELECT id, node_id, backend_mode, runtime, lane, snapshot_sha256 FROM attempts WHERE id = ? AND kind = ? AND status = ?`, physicalAttemptID, KindPhysical, StatusSucceeded).Scan(&source.ID, &source.NodeID, &source.BackendMode, &source.Runtime, &source.Lane, &source.SnapshotSHA256)
	if err != nil {
		return Attempt{}, err
	}
	now := time.Now().UTC()
	attempt := Attempt{ID: uuid.NewString(), NodeID: store.nodeID, Kind: KindReuse, Status: StatusReused, BackendMode: source.BackendMode, Runtime: source.Runtime, Lane: source.Lane, SnapshotSHA256: source.SnapshotSHA256, PhysicalAttemptID: source.ID, StartedAt: now, FinishedAt: now}
	_, err = store.db.ExecContext(ctx, `INSERT INTO attempts (id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, physical_attempt_id, started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', '', 0, 0)`, attempt.ID, attempt.NodeID, attempt.Kind, attempt.Status, attempt.BackendMode, attempt.Runtime, attempt.Lane, attempt.SnapshotSHA256, attempt.PhysicalAttemptID, now.UnixMilli(), now.UnixMilli())
	return attempt, err
}

func (store *Store) List(ctx context.Context, limit int, before int64) ([]Attempt, error) {
	if store == nil {
		return nil, nil
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	if before <= 0 {
		before = time.Now().Add(365 * 24 * time.Hour).UnixMilli()
	}
	rows, err := store.db.QueryContext(ctx, `SELECT id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, COALESCE(physical_attempt_id, ''), started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated FROM attempts WHERE started_at < ? ORDER BY started_at DESC, id DESC LIMIT ?`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := []Attempt{}
	for rows.Next() {
		attempt, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}

func (store *Store) Detail(ctx context.Context, attemptID string) (Detail, error) {
	var detail Detail
	if store == nil {
		return detail, sql.ErrNoRows
	}
	attempt, err := scanAttempt(store.db.QueryRowContext(ctx, `SELECT id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, COALESCE(physical_attempt_id, ''), started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated FROM attempts WHERE id = ?`, attemptID))
	if err != nil {
		return detail, err
	}
	detail.Attempt = attempt
	if err := store.db.QueryRowContext(ctx, `SELECT sha256, payload FROM snapshots WHERE sha256 = ?`, attempt.SnapshotSHA256).Scan(&detail.Snapshot.SHA256, &detail.Snapshot.JSON); err != nil {
		return Detail{}, err
	}
	rows, err := store.db.QueryContext(ctx, `SELECT role, position, sha256 FROM snapshot_assets WHERE snapshot_sha256 = ? ORDER BY role, position`, attempt.SnapshotSHA256)
	if err != nil {
		return Detail{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var asset Asset
		if err := rows.Scan(&asset.Role, &asset.Position, &asset.SHA256); err != nil {
			return Detail{}, err
		}
		detail.Assets = append(detail.Assets, asset)
	}
	return detail, rows.Err()
}

func (store *Store) Output(ctx context.Context, attemptID string, after int64, limit int) (OutputPage, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	var physicalAttemptID string
	if err := store.db.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(physical_attempt_id, ''), id) FROM attempts WHERE id = ?`, attemptID).Scan(&physicalAttemptID); err != nil {
		return OutputPage{}, err
	}
	rows, err := store.db.QueryContext(ctx, `SELECT sequence, stream, offset_ns, payload FROM output_chunks WHERE physical_attempt_id = ? AND sequence > ? ORDER BY sequence LIMIT ?`, physicalAttemptID, after, limit+1)
	if err != nil {
		return OutputPage{}, err
	}
	defer rows.Close()
	page := OutputPage{}
	for rows.Next() {
		var chunk Chunk
		var offset int64
		if err := rows.Scan(&chunk.Sequence, &chunk.Stream, &offset, &chunk.Payload); err != nil {
			return OutputPage{}, err
		}
		if len(page.Chunks) == limit {
			page.NextSequence = page.Chunks[len(page.Chunks)-1].Sequence
			break
		}
		chunk.Offset = time.Duration(offset)
		page.Chunks = append(page.Chunks, chunk)
	}
	return page, rows.Err()
}

func insertSnapshot(ctx context.Context, transaction *sql.Tx, snapshot Snapshot, now time.Time) error {
	if snapshot.SHA256 == "" || len(snapshot.JSON) == 0 {
		return fmt.Errorf("load capture snapshot is required")
	}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO snapshots (sha256, payload, created_at) VALUES (?, ?, ?) ON CONFLICT(sha256) DO NOTHING`, snapshot.SHA256, snapshot.JSON, now.UnixMilli()); err != nil {
		return err
	}
	for _, asset := range snapshot.Assets {
		if _, err := transaction.ExecContext(ctx, `INSERT INTO snapshot_assets (snapshot_sha256, role, position, sha256) VALUES (?, ?, ?, ?) ON CONFLICT(snapshot_sha256, role, position) DO NOTHING`, snapshot.SHA256, asset.Role, asset.Position, asset.SHA256); err != nil {
			return err
		}
	}
	return nil
}

func migrate(ctx context.Context, database *sql.DB) error {
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS snapshots (sha256 TEXT PRIMARY KEY, payload BLOB NOT NULL, created_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS snapshot_assets (snapshot_sha256 TEXT NOT NULL, role TEXT NOT NULL, position INTEGER NOT NULL, sha256 TEXT NOT NULL, PRIMARY KEY (snapshot_sha256, role, position), FOREIGN KEY (snapshot_sha256) REFERENCES snapshots(sha256) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS attempts (id TEXT PRIMARY KEY, node_id TEXT NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, backend_mode TEXT NOT NULL, runtime TEXT NOT NULL, lane TEXT NOT NULL, snapshot_sha256 TEXT NOT NULL, physical_attempt_id TEXT, started_at INTEGER NOT NULL, finished_at INTEGER NOT NULL DEFAULT 0, duration_ms INTEGER NOT NULL DEFAULT 0, failure_class TEXT NOT NULL DEFAULT '', failure_message TEXT NOT NULL DEFAULT '', captured_bytes INTEGER NOT NULL DEFAULT 0, truncated INTEGER NOT NULL DEFAULT 0, FOREIGN KEY (snapshot_sha256) REFERENCES snapshots(sha256), FOREIGN KEY (physical_attempt_id) REFERENCES attempts(id))`,
		`CREATE TABLE IF NOT EXISTS output_chunks (physical_attempt_id TEXT NOT NULL, sequence INTEGER NOT NULL, stream TEXT NOT NULL, offset_ns INTEGER NOT NULL, payload BLOB NOT NULL, PRIMARY KEY (physical_attempt_id, sequence), FOREIGN KEY (physical_attempt_id) REFERENCES attempts(id) ON DELETE CASCADE)`,
		`CREATE INDEX IF NOT EXISTS attempts_node_started ON attempts(node_id, started_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS attempts_status_started ON attempts(status, started_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS attempts_backend_started ON attempts(backend_mode, started_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS attempts_snapshot_started ON attempts(snapshot_sha256, started_at DESC, id DESC)`,
	}
	for _, statement := range statements {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	_, err := database.ExecContext(ctx, `UPDATE attempts SET status = ?, finished_at = started_at, duration_ms = 0, failure_class = 'interrupted', failure_message = 'router stopped before load completion' WHERE status = ?`, StatusInterrupted, StatusLoading)
	return err
}

func scanAttempt(scanner interface{ Scan(...any) error }) (Attempt, error) {
	var attempt Attempt
	var kind string
	var status string
	var started int64
	var finished int64
	var truncated int
	err := scanner.Scan(&attempt.ID, &attempt.NodeID, &kind, &status, &attempt.BackendMode, &attempt.Runtime, &attempt.Lane, &attempt.SnapshotSHA256, &attempt.PhysicalAttemptID, &started, &finished, &attempt.DurationMS, &attempt.FailureClass, &attempt.FailureMessage, &attempt.CapturedBytes, &truncated)
	if err != nil {
		return Attempt{}, err
	}
	attempt.Kind = Kind(kind)
	attempt.Status = Status(status)
	attempt.StartedAt = time.UnixMilli(started).UTC()
	if finished > 0 {
		attempt.FinishedAt = time.UnixMilli(finished).UTC()
	}
	attempt.Truncated = truncated != 0
	return attempt, nil
}

func sanitizeChunks(chunks []Chunk, redactions map[string]string) []Chunk {
	if len(chunks) == 0 {
		return nil
	}
	keys := replacementKeys(redactions)
	var combined []byte
	owners := make([]int, 0)
	for index, chunk := range chunks {
		combined = append(combined, chunk.Payload...)
		for range chunk.Payload {
			owners = append(owners, index)
		}
	}
	sanitized := make([][]byte, len(chunks))
	for position := 0; position < len(combined); {
		matched := ""
		for _, key := range keys {
			if bytes.HasPrefix(combined[position:], []byte(key)) {
				matched = key
				break
			}
		}
		owner := owners[position]
		if matched != "" {
			sanitized[owner] = append(sanitized[owner], redactions[matched]...)
			position += len(matched)
			continue
		}
		sanitized[owner] = append(sanitized[owner], combined[position])
		position++
	}
	result := make([]Chunk, 0, len(chunks))
	for index, payload := range sanitized {
		if len(payload) == 0 {
			continue
		}
		chunk := chunks[index]
		chunk.Payload = payload
		result = append(result, chunk)
	}
	return result
}

func captureRedactions(redactions map[string]string, secrets []string) map[string]string {
	result := make(map[string]string, len(redactions)+len(secrets))
	for value, replacement := range redactions {
		result[value] = replacement
	}
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); secret != "" {
			result[secret] = "[REDACTED]"
		}
	}
	return result
}

func replacementKeys(redactions map[string]string) []string {
	keys := make([]string, 0, len(redactions))
	for key := range redactions {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(left int, right int) bool { return len(keys[left]) > len(keys[right]) })
	return keys
}

func failureFields(err error, redactions map[string]string) (string, string) {
	if err == nil {
		return "", ""
	}
	message := err.Error()
	for _, key := range replacementKeys(redactions) {
		message = strings.ReplaceAll(message, key, redactions[key])
	}
	return fmt.Sprintf("%T", err), message
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

func (store *Store) databasePath() string {
	var path string
	_ = store.db.QueryRow(`PRAGMA database_list`).Scan(new(int), new(string), &path)
	return path
}
