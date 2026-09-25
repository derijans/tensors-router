package offloaddecisions

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	bufferLimit          = 10000
	defaultFlushInterval = time.Second
	defaultRetention     = 30 * 24 * time.Hour
	pruneInterval        = time.Minute
)

type StoreConfig struct {
	NodeID        string
	DB            *sql.DB
	ReadDB        *sql.DB
	Retention     time.Duration
	FlushInterval time.Duration
	Logger        *log.Logger
}

type Store struct {
	writer        *sql.DB
	reader        *sql.DB
	nodeID        string
	retention     time.Duration
	flushInterval time.Duration
	logger        *log.Logger

	mu       sync.Mutex
	buffer   []Record
	prunedAt time.Time

	closed    chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

func NewStore(config StoreConfig) (*Store, error) {
	if config.DB == nil || config.ReadDB == nil {
		return nil, fmt.Errorf("offload decision database handles are required")
	}
	nodeID := strings.TrimSpace(config.NodeID)
	if nodeID == "" {
		nodeID = "local"
	}
	retention := config.Retention
	if retention <= 0 {
		retention = defaultRetention
	}
	flushInterval := config.FlushInterval
	if flushInterval <= 0 {
		flushInterval = defaultFlushInterval
	}
	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}
	store := &Store{
		writer:        config.DB,
		reader:        config.ReadDB,
		nodeID:        nodeID,
		retention:     retention,
		flushInterval: flushInterval,
		logger:        logger,
		closed:        make(chan struct{}),
		done:          make(chan struct{}),
	}
	go store.flushLoop()
	return store, nil
}

func (store *Store) Record(record Record) {
	if store == nil {
		return
	}
	if record.RecordedAt.IsZero() {
		record.RecordedAt = time.Now()
	}
	if record.NodeID == "" {
		record.NodeID = store.nodeID
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.buffer) >= bufferLimit {
		store.buffer = store.buffer[1:]
	}
	store.buffer = append(store.buffer, record)
}

func (store *Store) Flush(ctx context.Context) error {
	if store == nil {
		return nil
	}
	records := store.takeBuffered()
	if err := store.write(ctx, records); err != nil {
		store.requeue(records)
		return err
	}
	return store.pruneExpired(ctx, time.Now())
}

func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	store.closeOnce.Do(func() { close(store.closed) })
	<-store.done
	return store.Flush(context.Background())
}

func (store *Store) Recent(ctx context.Context, limit int) ([]Record, error) {
	if store == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 200
	}
	rows, err := store.reader.QueryContext(ctx, `SELECT `+recordColumns+` FROM offload_decisions ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (store *Store) flushLoop() {
	defer close(store.done)
	ticker := time.NewTicker(store.flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-store.closed:
			return
		case <-ticker.C:
			if err := store.Flush(context.Background()); err != nil {
				store.logger.Printf("offload decision flush failed: %v", err)
			}
		}
	}
}

func (store *Store) takeBuffered() []Record {
	store.mu.Lock()
	defer store.mu.Unlock()
	records := store.buffer
	store.buffer = nil
	return records
}

func (store *Store) requeue(records []Record) {
	store.mu.Lock()
	defer store.mu.Unlock()
	combined := append(records, store.buffer...)
	if len(combined) > bufferLimit {
		combined = combined[len(combined)-bufferLimit:]
	}
	store.buffer = combined
}

const recordColumns = `recorded_at, node_id, kind, trigger, lane,
	owner_node_id, owner_model_id, helper_node_id, helper_model_id,
	outcome, reason, pending_count, backlog_count,
	keep_ms, switch_ms, service_ms, owner_job_ms, service_source,
	helper_idle_ms, slots, lent_out, borrowed_ahead, wait_ms`

func (store *Store) write(ctx context.Context, records []Record) error {
	if len(records) == 0 {
		return nil
	}
	transaction, err := store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	statement, err := transaction.PrepareContext(ctx, `INSERT INTO offload_decisions (`+recordColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, record := range records {
		if _, err := statement.ExecContext(ctx,
			record.RecordedAt.UnixMilli(), record.NodeID, string(record.Kind), record.Trigger, record.Lane,
			record.OwnerNodeID, record.OwnerModelID, record.HelperNodeID, record.HelperModelID,
			record.Outcome, record.Reason, record.PendingCount, record.BacklogCount,
			record.KeepMS, record.SwitchMS, record.ServiceMS, record.OwnerJobMS, record.ServiceSource,
			record.HelperIdleMS, record.Slots, record.LentOut, record.BorrowedAhead, record.WaitMS,
		); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (store *Store) pruneExpired(ctx context.Context, now time.Time) error {
	store.mu.Lock()
	due := now.Sub(store.prunedAt) >= pruneInterval
	if due {
		store.prunedAt = now
	}
	store.mu.Unlock()
	if !due {
		return nil
	}
	_, err := store.writer.ExecContext(ctx, `DELETE FROM offload_decisions WHERE recorded_at < ?`, now.Add(-store.retention).UnixMilli())
	return err
}

func scanRecord(rows *sql.Rows) (Record, error) {
	var record Record
	var recordedAt int64
	var kind string
	err := rows.Scan(
		&recordedAt, &record.NodeID, &kind, &record.Trigger, &record.Lane,
		&record.OwnerNodeID, &record.OwnerModelID, &record.HelperNodeID, &record.HelperModelID,
		&record.Outcome, &record.Reason, &record.PendingCount, &record.BacklogCount,
		&record.KeepMS, &record.SwitchMS, &record.ServiceMS, &record.OwnerJobMS, &record.ServiceSource,
		&record.HelperIdleMS, &record.Slots, &record.LentOut, &record.BorrowedAhead, &record.WaitMS,
	)
	record.RecordedAt = time.UnixMilli(recordedAt)
	record.Kind = Kind(kind)
	return record, err
}
