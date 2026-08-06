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
	if query.Limit < 1 || query.Limit > 500 {
		query.Limit = 100
	}
	statement := `SELECT id, node_id, kind, status, backend_mode, runtime, lane, snapshot_sha256, COALESCE(physical_attempt_id, ''), started_at, finished_at, duration_ms, failure_class, failure_message, captured_bytes, truncated FROM attempts WHERE 1 = 1`
	arguments := []any{}
	if query.BeforeStartedMS > 0 {
		if query.BeforeID == "" {
			statement += ` AND started_at < ?`
			arguments = append(arguments, query.BeforeStartedMS)
		} else {
			statement += ` AND (started_at < ? OR (started_at = ? AND id < ?))`
			arguments = append(arguments, query.BeforeStartedMS, query.BeforeStartedMS, query.BeforeID)
		}
	}
	if query.FromMS > 0 {
		statement += ` AND started_at >= ?`
		arguments = append(arguments, query.FromMS)
	}
	if query.ToMS > 0 {
		statement += ` AND started_at <= ?`
		arguments = append(arguments, query.ToMS)
	}
	if query.Status != "" {
		if !validStatus(query.Status) {
			return nil, fmt.Errorf("invalid load capture status %q", query.Status)
		}
		statement += ` AND status = ?`
		arguments = append(arguments, query.Status)
	}
	if query.Kind != "" {
		if query.Kind != KindPhysical && query.Kind != KindReuse {
			return nil, fmt.Errorf("invalid load capture kind %q", query.Kind)
		}
		statement += ` AND kind = ?`
		arguments = append(arguments, query.Kind)
	}
	if backendMode := strings.TrimSpace(query.BackendMode); backendMode != "" {
		statement += ` AND backend_mode = ?`
		arguments = append(arguments, backendMode)
	}
	statement += ` ORDER BY started_at DESC, id DESC LIMIT ?`
	arguments = append(arguments, query.Limit)
	rows, err := store.db.QueryContext(ctx, statement, arguments...)
	if err != nil {
		return nil, err
	}
	attempts := []Attempt{}
	for rows.Next() {
		attempt, err := scanAttempt(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range attempts {
		assetRows, err := store.db.QueryContext(ctx, `SELECT role, sha256 FROM snapshot_assets WHERE snapshot_sha256 = ? ORDER BY role, position`, attempts[index].SnapshotSHA256)
		if err != nil {
			return nil, err
		}
		for assetRows.Next() {
			var role string
			var hash string
			if err := assetRows.Scan(&role, &hash); err != nil {
				_ = assetRows.Close()
				return nil, err
			}
			attempts[index].ModelHashes = append(attempts[index].ModelHashes, role+":"+hash)
		}
		if err := assetRows.Close(); err != nil {
			return nil, err
		}
	}
	return attempts, nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusLoading, StatusSucceeded, StatusFailed, StatusInterrupted, StatusReused:
		return true
	default:
		return false
	}
}
