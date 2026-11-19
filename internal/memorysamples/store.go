package memorysamples

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"tensors-router/internal/memoryfit"
)

const (
	SamplesPerCombo        = 20
	siblingSampleLimit     = 200
	architectureSampleCap  = 200
	sampleColumnsForSelect = "context_size, vram_mb, ram_mb, predicted_gpu_mb"
)

type Record struct {
	Combo          memoryfit.Combo
	Architecture   string
	VRAMMB         int64
	RAMMB          int64
	PredictedGPUMB float64
}

type Store struct {
	writer *sql.DB
	reader *sql.DB
	now    func() time.Time
}

func NewStore(writer *sql.DB, reader *sql.DB) (*Store, error) {
	if writer == nil || reader == nil {
		return nil, errors.New("memory sample database handles are required")
	}
	return &Store{writer: writer, reader: reader, now: time.Now}, nil
}

func (store *Store) Record(ctx context.Context, record Record) error {
	if !record.Combo.Learnable() {
		return nil
	}
	tx, err := store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	comboKey := record.Combo.Key()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO memory_fit_samples (combo_key, placement_key, node_id, architecture, context_size, vram_mb, ram_mb, predicted_gpu_mb, recorded_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		comboKey, record.Combo.PlacementKey(), record.Combo.NodeID, record.Architecture, record.Combo.ContextSize,
		max(0, record.VRAMMB), max(0, record.RAMMB), record.PredictedGPUMB, store.now().UnixMilli(),
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM memory_fit_samples WHERE combo_key = ? AND id NOT IN (
			SELECT id FROM memory_fit_samples WHERE combo_key = ? ORDER BY id DESC LIMIT ?
		)`, comboKey, comboKey, SamplesPerCombo,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *Store) EstimateInput(ctx context.Context, target memoryfit.Combo, architecture string) (memoryfit.EstimateInput, error) {
	input := memoryfit.EstimateInput{Target: target}
	if !target.Learnable() {
		return input, nil
	}
	var err error
	if input.Exact, err = store.query(ctx, `combo_key = ?`, SamplesPerCombo, target.Key()); err != nil {
		return input, err
	}
	if input.Siblings, err = store.query(ctx, `placement_key = ? AND combo_key <> ?`, siblingSampleLimit, target.PlacementKey(), target.Key()); err != nil {
		return input, err
	}
	if architecture == "" {
		return input, nil
	}
	input.Architecture, err = store.query(ctx, `node_id = ? AND architecture = ? AND placement_key <> ? AND predicted_gpu_mb > 0`, architectureSampleCap, target.NodeID, architecture, target.PlacementKey())
	return input, err
}

func (store *Store) query(ctx context.Context, where string, limit int, arguments ...any) ([]memoryfit.Sample, error) {
	rows, err := store.reader.QueryContext(ctx, `SELECT `+sampleColumnsForSelect+` FROM memory_fit_samples WHERE `+where+` ORDER BY id DESC LIMIT ?`, append(arguments, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var samples []memoryfit.Sample
	for rows.Next() {
		var sample memoryfit.Sample
		if err := rows.Scan(&sample.ContextSize, &sample.VRAMMB, &sample.RAMMB, &sample.PredictedGPUMB); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}
