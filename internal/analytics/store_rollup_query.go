package analytics

import (
	"context"
	"time"
)

func (store *Store) queryUsesRollups(query Query) bool {
	cutoff := store.rawRetentionCutoff(time.Now())
	return cutoff > 0 && query.StartMS < cutoff
}

func (store *Store) queryRollupSummary(ctx context.Context, query Query) (Summary, error) {
	where, args := dailyRollupWhere(query)
	row := store.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(request_count), 0),
		COALESCE(SUM(success_count), 0),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(audio_tokens), 0),
		COALESCE(CAST(SUM(duration_ms_total) AS REAL) / NULLIF(SUM(request_count), 0), 0),
		COALESCE(SUM(tokens_per_second_sum) / NULLIF(SUM(tokens_per_second_count), 0), 0),
		COALESCE(SUM(load_count), 0),
		COALESCE(CAST(SUM(load_duration_ms_total) AS REAL) / NULLIF(SUM(load_count), 0), 0),
		COALESCE(MAX(vram_peak_mb), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(vram_total_mb), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_rollups `+where, args...)
	var summary Summary
	if err := row.Scan(
		&summary.RequestCount,
		&summary.SuccessCount,
		&summary.InputTokens,
		&summary.OutputTokens,
		&summary.TotalTokens,
		&summary.ImageCount,
		&summary.AudioSeconds,
		&summary.AudioTokens,
		&summary.AverageDuration,
		&summary.AverageTokensPS,
		&summary.LoadCount,
		&summary.AverageLoadMS,
		&summary.VRAMPeakMB,
		&summary.VRAMPeakPercent,
		&summary.VRAMTotalMB,
		&summary.ModelVRAMMB,
	); err != nil {
		return Summary{}, err
	}
	summary.FailureCount = summary.RequestCount - summary.SuccessCount
	return summary, nil
}

func (store *Store) queryRollupSections(ctx context.Context, query Query) ([]SectionUsage, error) {
	where, args := dailyRollupWhere(query)
	rows, err := store.db.QueryContext(ctx, `SELECT
		section,
		COALESCE(SUM(request_count), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(load_count), 0),
		COALESCE(MAX(vram_peak_mb), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_rollups `+where+`
		GROUP BY section
		ORDER BY COALESCE(SUM(request_count), 0) DESC, section`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SectionUsage{}
	for rows.Next() {
		var item SectionUsage
		if err := rows.Scan(&item.Section, &item.RequestCount, &item.TotalTokens, &item.ImageCount, &item.AudioSeconds, &item.LoadCount, &item.VRAMPeakMB, &item.VRAMPeakPct, &item.ModelVRAMMB); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) queryRollupModels(ctx context.Context, query Query) ([]ModelUsage, error) {
	where, args := dailyRollupWhere(query)
	rows, err := store.db.QueryContext(ctx, `SELECT
		node_id,
		model_id,
		COALESCE(SUM(request_count), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(load_count), 0),
		COALESCE(CAST(SUM(load_duration_ms_total) AS REAL) / NULLIF(SUM(load_count), 0), 0),
		COALESCE(MAX(vram_peak_mb), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_rollups `+where+`
		GROUP BY node_id, model_id
		ORDER BY COALESCE(SUM(request_count), 0) DESC, model_id
		LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ModelUsage{}
	for rows.Next() {
		var item ModelUsage
		if err := rows.Scan(&item.NodeID, &item.ModelID, &item.RequestCount, &item.TotalTokens, &item.ImageCount, &item.AudioSeconds, &item.LoadCount, &item.AverageLoadMS, &item.VRAMPeakMB, &item.VRAMPeakPct, &item.ModelVRAMMB); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) queryRollupNodes(ctx context.Context, query Query) ([]NodeUsage, error) {
	where, args := dailyRollupWhere(query)
	rows, err := store.db.QueryContext(ctx, `SELECT
		node_id,
		COALESCE(SUM(request_count), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(load_count), 0),
		COALESCE(CAST(SUM(load_duration_ms_total) AS REAL) / NULLIF(SUM(load_count), 0), 0),
		COALESCE(MAX(vram_peak_mb), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_rollups `+where+`
		GROUP BY node_id
		ORDER BY node_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NodeUsage{}
	for rows.Next() {
		var item NodeUsage
		if err := rows.Scan(&item.NodeID, &item.RequestCount, &item.TotalTokens, &item.ImageCount, &item.AudioSeconds, &item.LoadCount, &item.AverageLoadMS, &item.VRAMPeakMB, &item.VRAMPeakPct, &item.ModelVRAMMB); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func dailyRollupWhere(query Query) (string, []any) {
	query.StartMS = dayStartMilliseconds(query.StartMS)
	query.EndMS = dayStartMilliseconds(query.EndMS)
	return rollupWhere(query, "day")
}

func dayStartMilliseconds(value int64) int64 {
	instant := time.UnixMilli(value).UTC()
	year, month, day := instant.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).UnixMilli()
}
