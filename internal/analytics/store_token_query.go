package analytics

import (
	"context"
	"time"
)

type TokenProfileSample struct {
	NodeID           string
	ModelID          string
	Count            int64
	SumRatio         float64
	SumRatioSquared  float64
	SumOutput        float64
	SumOutputSquared float64
}

func (store *Store) TokenProfileSamples(ctx context.Context, window time.Duration, now time.Time) ([]TokenProfileSample, error) {
	if store == nil {
		return nil, nil
	}
	if err := store.Flush(ctx); err != nil {
		return nil, err
	}
	since := now.Add(-window).UnixMilli()
	rows, err := store.reader.QueryContext(ctx, `SELECT model_id, COUNT(*),
			SUM(ratio), SUM(ratio * ratio),
			SUM(output), SUM(output * output)
		FROM (
			SELECT model_id,
			       CAST(prompt_bytes AS REAL) / CAST(input_tokens AS REAL) AS ratio,
			       CAST(output_tokens AS REAL) AS output
			FROM analytics_events
			WHERE event_type = ? AND success = 1 AND section = ? AND finished_at >= ?
				AND prompt_bytes > 0 AND input_tokens > 0 AND output_tokens > 0
		)
		GROUP BY model_id`, EventTypeRequest, SectionLLM, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var samples []TokenProfileSample
	for rows.Next() {
		sample := TokenProfileSample{NodeID: store.nodeID}
		if err := rows.Scan(
			&sample.ModelID,
			&sample.Count,
			&sample.SumRatio,
			&sample.SumRatioSquared,
			&sample.SumOutput,
			&sample.SumOutputSquared,
		); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}
