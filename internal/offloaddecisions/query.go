package offloaddecisions

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

const (
	defaultQueryLimit = 200
	maximumQueryLimit = 5000
)

type Filter struct {
	Since   time.Time
	Lane    string
	Outcome string
	Limit   int
}

func (filter Filter) boundedLimit() int {
	switch {
	case filter.Limit <= 0:
		return defaultQueryLimit
	case filter.Limit > maximumQueryLimit:
		return maximumQueryLimit
	default:
		return filter.Limit
	}
}

func (store *Store) Query(ctx context.Context, filter Filter) ([]Record, error) {
	records := []Record{}
	if store == nil {
		return records, nil
	}
	conditions := []string{"1 = 1"}
	var arguments []any
	if !filter.Since.IsZero() {
		conditions = append(conditions, "recorded_at >= ?")
		arguments = append(arguments, filter.Since.UnixMilli())
	}
	if filter.Lane != "" {
		conditions = append(conditions, "lane = ?")
		arguments = append(arguments, filter.Lane)
	}
	if filter.Outcome != "" {
		conditions = append(conditions, "outcome = ?")
		arguments = append(arguments, filter.Outcome)
	}
	arguments = append(arguments, filter.boundedLimit())
	rows, err := store.reader.QueryContext(ctx, `SELECT id, `+recordColumns+` FROM offload_decisions
		WHERE `+strings.Join(conditions, " AND ")+` ORDER BY id DESC LIMIT ?`, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func scanRecord(rows *sql.Rows) (Record, error) {
	var record Record
	var recordedAt int64
	var kind string
	err := rows.Scan(
		&record.ID, &recordedAt, &record.NodeID, &kind, &record.Trigger, &record.Lane,
		&record.OwnerNodeID, &record.OwnerModelID, &record.HelperNodeID, &record.HelperModelID,
		&record.Outcome, &record.Reason, &record.PendingCount, &record.BacklogCount,
		&record.KeepMS, &record.SwitchMS, &record.ServiceMS, &record.OwnerJobMS, &record.ServiceSource,
		&record.HelperIdleMS, &record.Slots, &record.LentOut, &record.BorrowedAhead, &record.WaitMS, &record.RouterVersion,
	)
	record.RecordedAt = time.UnixMilli(recordedAt)
	record.Kind = Kind(kind)
	return record, err
}
