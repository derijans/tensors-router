package analytics

import (
	"context"
	"time"
)

const textPrefillExpression = `CAST(input_tokens AS REAL)`
const textDecodeExpression = `CAST(output_tokens AS REAL)`

func TextWork(inputTokens int64, outputTokens int64) (prefill float64, decode float64, ok bool) {
	if inputTokens <= 0 || outputTokens <= 0 {
		return 0, 0, false
	}
	return float64(inputTokens), float64(outputTokens), true
}

func (store *Store) TextCostSamples(ctx context.Context, window time.Duration, now time.Time) ([]CostSample, error) {
	if store == nil {
		return nil, nil
	}
	if err := store.Flush(ctx); err != nil {
		return nil, err
	}
	since := now.Add(-window).UnixMilli()
	return store.textCostSamples(ctx, since)
}

func (store *Store) textCostSamples(ctx context.Context, since int64) ([]CostSample, error) {
	rows, err := store.reader.QueryContext(ctx, `SELECT model_id, COUNT(*),
			SUM(duration),
			SUM(prefill), SUM(decode),
			SUM(prefill * duration), SUM(decode * duration),
			SUM(prefill * prefill), SUM(decode * decode), SUM(prefill * decode)
		FROM (
			SELECT model_id,
			       CAST(duration_ms AS REAL) AS duration,
			       `+textPrefillExpression+` AS prefill,
			       `+textDecodeExpression+` AS decode
			FROM analytics_events
			WHERE event_type = ? AND success = 1 AND section = ? AND finished_at >= ?
				AND duration_ms > 0 AND input_tokens > 0 AND output_tokens > 0
		)
		GROUP BY model_id`, EventTypeRequest, SectionLLM, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var samples []CostSample
	for rows.Next() {
		sample := CostSample{NodeID: store.nodeID, Section: SectionLLM, Arity: 2}
		var sumPrefillDecode float64
		if err := rows.Scan(
			&sample.ModelID,
			&sample.Count,
			&sample.SumDuration,
			&sample.SumWork[0],
			&sample.SumWork[1],
			&sample.SumWorkDuration[0],
			&sample.SumWorkDuration[1],
			&sample.SumWorkProduct[0][0],
			&sample.SumWorkProduct[1][1],
			&sumPrefillDecode,
		); err != nil {
			return nil, err
		}
		sample.SumWorkProduct[0][1] = sumPrefillDecode
		sample.SumWorkProduct[1][0] = sumPrefillDecode
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}
