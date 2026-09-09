package loadcapture

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
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
	NodeID string
	DB     *sql.DB
	ReadDB *sql.DB
	Logger *log.Logger
}

type Store struct {
	writer *sql.DB
	reader *sql.DB
	nodeID string
	logger *log.Logger
}

func NewStore(config StoreConfig) (*Store, error) {
	if config.DB == nil || config.ReadDB == nil {
		return nil, fmt.Errorf("load capture database handles are required")
	}
	if err := reconcileInterruptedAttempts(context.Background(), config.DB); err != nil {
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
	return &Store{writer: config.DB, reader: config.ReadDB, nodeID: nodeID, logger: logger}, nil
}

func reconcileInterruptedAttempts(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `UPDATE load_capture_attempts SET status = ?, finished_at = started_at, duration_ms = 0, failure_class = 'interrupted', failure_message = 'router stopped before load completion' WHERE status = ?`, StatusInterrupted, StatusLoading)
	return err
}

func (store *Store) BeginPhysical(ctx context.Context, snapshot Snapshot, backendMode string, runtime string, lane string) (Attempt, error) {
	if store == nil {
		return Attempt{}, nil
	}
	now := time.Now().UTC()
	attempt := Attempt{ID: uuid.NewString(), NodeID: store.nodeID, Kind: KindPhysical, Status: StatusLoading, BackendMode: strings.TrimSpace(backendMode), Runtime: strings.TrimSpace(runtime), Lane: strings.TrimSpace(lane), SnapshotSHA256: snapshot.SHA256, StartedAt: now}
	transaction, err := store.writer.BeginTx(ctx, nil)
	if err != nil {
		return Attempt{}, err
	}
	if err = insertSnapshot(ctx, transaction, snapshot, now); err == nil {
		_, err = transaction.ExecContext(ctx, `INSERT INTO load_capture_attempts (id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, physical_attempt_id, started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, 0, 0, '', '', 0, 0)`, attempt.ID, attempt.NodeID, attempt.Kind, attempt.Status, attempt.BackendMode, attempt.Runtime, attempt.Lane, attempt.SnapshotSHA256, attempt.StartedAt.UnixMilli())
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
	transaction, err := store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err = transaction.ExecContext(ctx, `UPDATE load_capture_attempts SET status = ?, finished_at = ?, duration_ms = ?, failure_class = ?, failure_message = ?, captured_bytes = ?, truncated = ? WHERE id = ? AND status = ?`, status, finished.UnixMilli(), finished.Sub(attempt.StartedAt).Milliseconds(), failureClass, failureMessage, capture.CapturedBytes, boolValue(capture.Truncated), attempt.ID, StatusLoading); err == nil {
		statement, prepareErr := transaction.PrepareContext(ctx, `INSERT INTO load_capture_output_chunks (physical_attempt_id, sequence, stream, offset_ns, payload) VALUES (?, ?, ?, ?, ?)`)
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
	return nil
}

func (store *Store) RecordReuse(ctx context.Context, physicalAttemptID string) (Attempt, error) {
	if store == nil || strings.TrimSpace(physicalAttemptID) == "" {
		return Attempt{}, nil
	}
	var source Attempt
	err := store.reader.QueryRowContext(ctx, `SELECT id, node_id, backend_mode, runtime, lane, snapshot_sha256 FROM load_capture_attempts WHERE id = ? AND kind = ? AND status = ?`, physicalAttemptID, KindPhysical, StatusSucceeded).Scan(&source.ID, &source.NodeID, &source.BackendMode, &source.Runtime, &source.Lane, &source.SnapshotSHA256)
	if err != nil {
		return Attempt{}, err
	}
	now := time.Now().UTC()
	attempt := Attempt{ID: uuid.NewString(), NodeID: store.nodeID, Kind: KindReuse, Status: StatusReused, BackendMode: source.BackendMode, Runtime: source.Runtime, Lane: source.Lane, SnapshotSHA256: source.SnapshotSHA256, PhysicalAttemptID: source.ID, StartedAt: now, FinishedAt: now}
	_, err = store.writer.ExecContext(ctx, `INSERT INTO load_capture_attempts (id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, physical_attempt_id, started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', '', 0, 0)`, attempt.ID, attempt.NodeID, attempt.Kind, attempt.Status, attempt.BackendMode, attempt.Runtime, attempt.Lane, attempt.SnapshotSHA256, attempt.PhysicalAttemptID, now.UnixMilli(), now.UnixMilli())
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
	rows, err := store.reader.QueryContext(ctx, `SELECT id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, COALESCE(physical_attempt_id, ''), started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated FROM load_capture_attempts WHERE started_at < ? ORDER BY started_at DESC, id DESC LIMIT ?`, before, limit)
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
	attempt, err := scanAttempt(store.reader.QueryRowContext(ctx, `SELECT id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, COALESCE(physical_attempt_id, ''), started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated FROM load_capture_attempts WHERE id = ?`, attemptID))
	if err != nil {
		return detail, err
	}
	detail.Attempt = attempt
	if err := store.reader.QueryRowContext(ctx, `SELECT sha256, payload FROM load_capture_snapshots WHERE sha256 = ?`, attempt.SnapshotSHA256).Scan(&detail.Snapshot.SHA256, &detail.Snapshot.JSON); err != nil {
		return Detail{}, err
	}
	rows, err := store.reader.QueryContext(ctx, `SELECT role, position, sha256 FROM load_capture_snapshot_assets WHERE snapshot_sha256 = ? ORDER BY role, position`, attempt.SnapshotSHA256)
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
	if err := store.reader.QueryRowContext(ctx, `SELECT COALESCE(NULLIF(physical_attempt_id, ''), id) FROM load_capture_attempts WHERE id = ?`, attemptID).Scan(&physicalAttemptID); err != nil {
		return OutputPage{}, err
	}
	rows, err := store.reader.QueryContext(ctx, `SELECT sequence, stream, offset_ns, payload FROM load_capture_output_chunks WHERE physical_attempt_id = ? AND sequence > ? ORDER BY sequence LIMIT ?`, physicalAttemptID, after, limit+1)
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
	if _, err := transaction.ExecContext(ctx, `INSERT INTO load_capture_snapshots (sha256, payload, created_at) VALUES (?, ?, ?) ON CONFLICT(sha256) DO NOTHING`, snapshot.SHA256, snapshot.JSON, now.UnixMilli()); err != nil {
		return err
	}
	for _, asset := range snapshot.Assets {
		if _, err := transaction.ExecContext(ctx, `INSERT INTO load_capture_snapshot_assets (snapshot_sha256, role, position, sha256) VALUES (?, ?, ?, ?) ON CONFLICT(snapshot_sha256, role, position) DO NOTHING`, snapshot.SHA256, asset.Role, asset.Position, asset.SHA256); err != nil {
			return err
		}
	}
	return nil
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
