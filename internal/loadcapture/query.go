package loadcapture

import (
	"context"
	"fmt"
	"strings"
)

type ListQuery struct {
	Limit           int
	BeforeStartedMS int64
	BeforeID        string
	FromMS          int64
	ToMS            int64
	Status          Status
	Kind            Kind
	BackendMode     string
}

func (store *Store) ListFiltered(ctx context.Context, query ListQuery) ([]Attempt, error) {
	if store == nil {
		return nil, nil
	}
	if err := query.validate(); err != nil {
		return nil, err
	}
	attempts, err := store.queryAttempts(ctx, query)
	if err != nil {
		return nil, err
	}
	for index := range attempts {
		attempts[index].ModelHashes, err = store.snapshotModelHashes(ctx, attempts[index].SnapshotSHA256)
		if err != nil {
			return nil, err
		}
	}
	return attempts, nil
}

func (query ListQuery) validate() error {
	if query.Status != "" && !validStatus(query.Status) {
		return fmt.Errorf("invalid load capture status %q", query.Status)
	}
	if query.Kind != "" && query.Kind != KindPhysical && query.Kind != KindReuse {
		return fmt.Errorf("invalid load capture kind %q", query.Kind)
	}
	return nil
}

func (query ListQuery) filterClause() (string, []any) {
	var clause strings.Builder
	arguments := []any{}
	addFilter := func(condition string, values ...any) {
		clause.WriteString(" AND " + condition)
		arguments = append(arguments, values...)
	}
	if query.BeforeStartedMS > 0 && query.BeforeID == "" {
		addFilter("started_at < ?", query.BeforeStartedMS)
	}
	if query.BeforeStartedMS > 0 && query.BeforeID != "" {
		addFilter("(started_at < ? OR (started_at = ? AND id < ?))", query.BeforeStartedMS, query.BeforeStartedMS, query.BeforeID)
	}
	if query.FromMS > 0 {
		addFilter("started_at >= ?", query.FromMS)
	}
	if query.ToMS > 0 {
		addFilter("started_at <= ?", query.ToMS)
	}
	if query.Status != "" {
		addFilter("status = ?", query.Status)
	}
	if query.Kind != "" {
		addFilter("kind = ?", query.Kind)
	}
	if backendMode := strings.TrimSpace(query.BackendMode); backendMode != "" {
		addFilter("backend_mode = ?", backendMode)
	}
	return clause.String(), arguments
}

func (query ListQuery) pageLimit() int {
	if query.Limit < 1 || query.Limit > 500 {
		return 100
	}
	return query.Limit
}

func (store *Store) queryAttempts(ctx context.Context, query ListQuery) ([]Attempt, error) {
	filter, arguments := query.filterClause()
	statement := `SELECT id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, COALESCE(physical_attempt_id, ''), started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated FROM load_capture_attempts WHERE 1 = 1` + filter + ` ORDER BY started_at DESC, id DESC LIMIT ?`
	rows, err := store.reader.QueryContext(ctx, statement, append(arguments, query.pageLimit())...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	attempts := []Attempt{}
	for rows.Next() {
		attempt, err := scanAttempt(rows)
		if err != nil {
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return attempts, rows.Close()
}

func (store *Store) snapshotModelHashes(ctx context.Context, snapshotSHA256 string) ([]string, error) {
	rows, err := store.reader.QueryContext(ctx, `SELECT role, sha256 FROM load_capture_snapshot_assets WHERE snapshot_sha256 = ? ORDER BY role, position`, snapshotSHA256)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var hashes []string
	for rows.Next() {
		var role string
		var hash string
		if err := rows.Scan(&role, &hash); err != nil {
			return nil, err
		}
		hashes = append(hashes, role+":"+hash)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hashes, rows.Close()
}

func validStatus(status Status) bool {
	switch status {
	case StatusLoading, StatusSucceeded, StatusFailed, StatusInterrupted, StatusReused:
		return true
	default:
		return false
	}
}
