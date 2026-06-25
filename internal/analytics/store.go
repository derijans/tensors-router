package analytics

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultBufferLimit = 10000
	flushTriggerSize   = 1000
)

type StoreConfig struct {
	NodeID        string
	DatabasePath  string
	FlushInterval time.Duration
	Logger        *log.Logger
}

type Store struct {
	db            *sql.DB
	nodeID        string
	flushInterval time.Duration
	bufferLimit   int
	logger        *log.Logger
	mu            sync.Mutex
	buffer        []Event
	flushSignal   chan struct{}
	closed        chan struct{}
	done          chan struct{}
	closeOnce     sync.Once
}

func NewStore(config StoreConfig) (*Store, error) {
	nodeID := strings.TrimSpace(config.NodeID)
	if nodeID == "" {
		nodeID = "local"
	}
	databasePath := strings.TrimSpace(config.DatabasePath)
	if databasePath == "" {
		return nil, fmt.Errorf("analytics database path is required")
	}
	if config.FlushInterval <= 0 {
		return nil, fmt.Errorf("analytics flush interval must be positive")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := migrate(context.Background(), db); err != nil {
		_ = db.Close()
		return nil, err
	}
	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}
	store := &Store{
		db:            db,
		nodeID:        nodeID,
		flushInterval: config.FlushInterval,
		bufferLimit:   defaultBufferLimit,
		logger:        logger,
		flushSignal:   make(chan struct{}, 1),
		closed:        make(chan struct{}),
		done:          make(chan struct{}),
	}
	go store.flushLoop()
	return store, nil
}

func (store *Store) Record(event Event) {
	if store == nil {
		return
	}
	event = store.normalizeEvent(event)
	store.mu.Lock()
	if len(store.buffer) >= store.bufferLimit {
		copy(store.buffer, store.buffer[1:])
		store.buffer[len(store.buffer)-1] = event
	} else {
		store.buffer = append(store.buffer, event)
	}
	shouldFlush := len(store.buffer) >= flushTriggerSize
	store.mu.Unlock()
	if shouldFlush {
		store.requestFlush()
	}
}

func (store *Store) Flush(ctx context.Context) error {
	if store == nil {
		return nil
	}
	events := store.takeBufferedEvents()
	if len(events) == 0 {
		return nil
	}
	if err := store.writeEvents(ctx, events); err != nil {
		store.requeue(events)
		return err
	}
	return nil
}

func (store *Store) Close(ctx context.Context) error {
	if store == nil {
		return nil
	}
	store.closeOnce.Do(func() {
		close(store.closed)
		<-store.done
	})
	flushErr := store.Flush(ctx)
	closeErr := store.db.Close()
	if flushErr != nil {
		return flushErr
	}
	return closeErr
}

func (store *Store) flushLoop() {
	ticker := time.NewTicker(store.flushInterval)
	defer ticker.Stop()
	defer close(store.done)
	for {
		select {
		case <-ticker.C:
			store.flushBackground()
		case <-store.flushSignal:
			store.flushBackground()
		case <-store.closed:
			return
		}
	}
}

func (store *Store) flushBackground() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := store.Flush(ctx); err != nil {
		store.logger.Printf("analytics flush failed: %v", err)
	}
}

func (store *Store) requestFlush() {
	select {
	case store.flushSignal <- struct{}{}:
	default:
	}
}

func (store *Store) takeBufferedEvents() []Event {
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.buffer) == 0 {
		return nil
	}
	events := append([]Event{}, store.buffer...)
	for index := range store.buffer {
		store.buffer[index] = Event{}
	}
	store.buffer = store.buffer[:0]
	return events
}

func (store *Store) requeue(events []Event) {
	store.mu.Lock()
	defer store.mu.Unlock()
	combined := make([]Event, 0, len(events)+len(store.buffer))
	combined = append(combined, events...)
	combined = append(combined, store.buffer...)
	if len(combined) > store.bufferLimit {
		combined = combined[len(combined)-store.bufferLimit:]
	}
	store.buffer = combined
}

func (store *Store) normalizeEvent(event Event) Event {
	event.NodeID = strings.TrimSpace(event.NodeID)
	if event.NodeID == "" {
		event.NodeID = store.nodeID
	}
	event.ModelID = strings.TrimSpace(event.ModelID)
	event.Section = strings.TrimSpace(event.Section)
	event.BackendMode = strings.TrimSpace(event.BackendMode)
	event.Route = strings.TrimSpace(event.Route)
	event.ImageType = strings.TrimSpace(event.ImageType)
	if event.Route == "" {
		event.Route = "unknown"
	}
	if event.Section == "" {
		event.Section = SectionLLM
	}
	if event.StartedAt.IsZero() {
		event.StartedAt = time.Now()
	}
	if event.FinishedAt.IsZero() {
		event.FinishedAt = event.StartedAt
	}
	if event.DurationMS == 0 {
		event.DurationMS = event.FinishedAt.Sub(event.StartedAt).Milliseconds()
	}
	if event.TotalTokens == 0 && (event.InputTokens > 0 || event.OutputTokens > 0) {
		event.TotalTokens = event.InputTokens + event.OutputTokens
	}
	if event.TokensPerSecond == 0 && event.OutputTokens > 0 && event.DurationMS > 0 {
		event.TokensPerSecond = float64(event.OutputTokens) / (float64(event.DurationMS) / 1000)
	}
	return event
}

func (store *Store) writeEvents(ctx context.Context, events []Event) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	insertEvent, err := tx.PrepareContext(ctx, `INSERT INTO analytics_events (
		node_id, model_id, section, backend_mode, route, status_code, success,
		started_at, finished_at, duration_ms, input_tokens, output_tokens,
		total_tokens, tokens_per_second, image_count, image_width, image_height,
		image_type, audio_seconds, audio_tokens
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insertEvent.Close()

	rollupStatement, err := tx.PrepareContext(ctx, rollupInsertSQL())
	if err != nil {
		return err
	}
	defer rollupStatement.Close()

	for _, event := range events {
		event = store.normalizeEvent(event)
		if _, err = insertEvent.ExecContext(ctx,
			event.NodeID,
			event.ModelID,
			event.Section,
			event.BackendMode,
			event.Route,
			event.StatusCode,
			boolInt(event.Success),
			event.StartedAt.UnixMilli(),
			event.FinishedAt.UnixMilli(),
			event.DurationMS,
			event.InputTokens,
			event.OutputTokens,
			event.TotalTokens,
			event.TokensPerSecond,
			event.ImageCount,
			event.ImageWidth,
			event.ImageHeight,
			event.ImageType,
			event.AudioSeconds,
			event.AudioTokens,
		); err != nil {
			return err
		}
		if err = store.writeRollups(ctx, rollupStatement, event); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (store *Store) writeRollups(ctx context.Context, statement *sql.Stmt, event Event) error {
	for _, period := range []string{"hour", "day"} {
		if _, err := statement.ExecContext(ctx,
			period,
			bucketStart(event.FinishedAt, period),
			event.NodeID,
			event.ModelID,
			event.Section,
			event.BackendMode,
			event.Route,
			1,
			boolInt(event.Success),
			event.DurationMS,
			event.InputTokens,
			event.OutputTokens,
			event.TotalTokens,
			event.TokensPerSecond,
			tokensPerSecondCount(event.TokensPerSecond),
			event.ImageCount,
			event.AudioSeconds,
			event.AudioTokens,
		); err != nil {
			return err
		}
	}
	return nil
}

func rollupInsertSQL() string {
	return `INSERT INTO analytics_rollups (
		period_kind, bucket_start, node_id, model_id, section, backend_mode, route,
		request_count, success_count, duration_ms_total, input_tokens, output_tokens,
		total_tokens, tokens_per_second_sum, tokens_per_second_count, image_count,
		audio_seconds, audio_tokens
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(period_kind, bucket_start, node_id, model_id, section, backend_mode, route)
	DO UPDATE SET
		request_count = request_count + excluded.request_count,
		success_count = success_count + excluded.success_count,
		duration_ms_total = duration_ms_total + excluded.duration_ms_total,
		input_tokens = input_tokens + excluded.input_tokens,
		output_tokens = output_tokens + excluded.output_tokens,
		total_tokens = total_tokens + excluded.total_tokens,
		tokens_per_second_sum = tokens_per_second_sum + excluded.tokens_per_second_sum,
		tokens_per_second_count = tokens_per_second_count + excluded.tokens_per_second_count,
		image_count = image_count + excluded.image_count,
		audio_seconds = audio_seconds + excluded.audio_seconds,
		audio_tokens = audio_tokens + excluded.audio_tokens`
}

func bucketStart(value time.Time, period string) int64 {
	value = value.UTC()
	if period == "hour" {
		return value.Truncate(time.Hour).UnixMilli()
	}
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).UnixMilli()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func tokensPerSecondCount(value float64) int {
	if value > 0 {
		return 1
	}
	return 0
}
