package analytics

import (
	"context"
	"strings"
	"time"
)

func DisabledResponse(query Query) Response {
	return Response{
		Enabled:     false,
		From:        query.StartMS,
		To:          query.EndMS,
		Granularity: Granularity(query),
		Filters:     emptyFilters(),
	}
}

func (store *Store) Query(ctx context.Context, query Query) (Response, error) {
	if store == nil {
		normalized, _ := NormalizeQuery(query, time.Now())
		return DisabledResponse(normalized), nil
	}
	normalized, err := NormalizeQuery(query, time.Now())
	if err != nil {
		return Response{}, err
	}
	if err := store.Flush(ctx); err != nil {
		store.logger.Printf("analytics flush before query failed: %v", err)
	}
	response := Response{
		Enabled:     true,
		From:        normalized.StartMS,
		To:          normalized.EndMS,
		Granularity: Granularity(normalized),
		Filters:     emptyFilters(),
	}
	if response.Filters, err = store.queryFilters(ctx, normalized); err != nil {
		return Response{}, err
	}
	if response.Summary, err = store.querySummary(ctx, normalized); err != nil {
		return Response{}, err
	}
	if response.Timeline, err = store.queryTimeline(ctx, normalized, response.Granularity); err != nil {
		return Response{}, err
	}
	if response.Sections, err = store.querySections(ctx, normalized); err != nil {
		return Response{}, err
	}
	if response.Models, err = store.queryModels(ctx, normalized); err != nil {
		return Response{}, err
	}
	if response.Nodes, err = store.queryNodes(ctx, normalized); err != nil {
		return Response{}, err
	}
	if response.Recent, err = store.queryRecent(ctx, normalized); err != nil {
		return Response{}, err
	}
	return response, nil
}

func (store *Store) queryFilters(ctx context.Context, query Query) (Filters, error) {
	query.NodeID = ""
	query.ModelID = ""
	query.Section = ""
	if store.queryUsesRollups(query) {
		return store.queryRollupFilters(ctx, query)
	}
	where, args := eventWhere(query)
	nodeIDs, err := store.queryDistinctValues(ctx, "SELECT DISTINCT node_id FROM analytics_events "+where+" AND node_id <> '' ORDER BY node_id", args)
	if err != nil {
		return Filters{}, err
	}
	modelIDs, err := store.queryDistinctValues(ctx, "SELECT DISTINCT model_id FROM analytics_events "+where+" AND model_id <> '' ORDER BY model_id", args)
	if err != nil {
		return Filters{}, err
	}
	return Filters{NodeIDs: nodeIDs, ModelIDs: modelIDs}, nil
}

func (store *Store) queryDistinctValues(ctx context.Context, statement string, args []any) ([]string, error) {
	rows, err := store.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func emptyFilters() Filters {
	return Filters{NodeIDs: []string{}, ModelIDs: []string{}}
}

func (store *Store) querySummary(ctx context.Context, query Query) (Summary, error) {
	if store.queryUsesRollups(query) {
		return store.queryRollupSummary(ctx, query)
	}
	where, args := eventWhere(query)
	row := store.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN event_type = 'request' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN event_type = 'request' THEN success ELSE 0 END), 0),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(audio_tokens), 0),
		COALESCE(AVG(CASE WHEN event_type = 'request' THEN duration_ms END), 0),
		COALESCE(AVG(CASE WHEN event_type = 'request' THEN NULLIF(tokens_per_second, 0) END), 0),
		COALESCE(SUM(CASE WHEN event_type = 'model_load' THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(CASE WHEN event_type = 'model_load' THEN duration_ms END), 0),
		COALESCE(MAX(CASE WHEN work_vram_max_mb > load_vram_after_mb THEN work_vram_max_mb ELSE load_vram_after_mb END), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(vram_total_mb), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_events `+where, args...)
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

func (store *Store) queryTimeline(ctx context.Context, query Query, granularity string) ([]Timeline, error) {
	where, args := rollupWhere(query, granularity)
	rows, err := store.db.QueryContext(ctx, `SELECT
		bucket_start,
		COALESCE(SUM(request_count), 0),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(load_count), 0),
		COALESCE(MAX(vram_peak_mb), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(vram_total_mb), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_rollups `+where+`
		GROUP BY bucket_start
		ORDER BY bucket_start`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Timeline
	for rows.Next() {
		var item Timeline
		if err := rows.Scan(
			&item.BucketStart,
			&item.RequestCount,
			&item.InputTokens,
			&item.OutputTokens,
			&item.TotalTokens,
			&item.ImageCount,
			&item.AudioSeconds,
			&item.LoadCount,
			&item.VRAMPeakMB,
			&item.VRAMPeakPct,
			&item.VRAMTotalMB,
			&item.ModelVRAMMB,
		); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) querySections(ctx context.Context, query Query) ([]SectionUsage, error) {
	if store.queryUsesRollups(query) {
		return store.queryRollupSections(ctx, query)
	}
	where, args := eventWhere(query)
	rows, err := store.db.QueryContext(ctx, `SELECT
		section,
		COALESCE(SUM(CASE WHEN event_type = 'request' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(CASE WHEN event_type = 'model_load' THEN 1 ELSE 0 END), 0),
		COALESCE(MAX(CASE WHEN work_vram_max_mb > load_vram_after_mb THEN work_vram_max_mb ELSE load_vram_after_mb END), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_events `+where+`
		GROUP BY section
		ORDER BY COALESCE(SUM(CASE WHEN event_type = 'request' THEN 1 ELSE 0 END), 0) DESC, section`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SectionUsage
	for rows.Next() {
		var item SectionUsage
		if err := rows.Scan(&item.Section, &item.RequestCount, &item.TotalTokens, &item.ImageCount, &item.AudioSeconds, &item.LoadCount, &item.VRAMPeakMB, &item.VRAMPeakPct, &item.ModelVRAMMB); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) queryModels(ctx context.Context, query Query) ([]ModelUsage, error) {
	if store.queryUsesRollups(query) {
		return store.queryRollupModels(ctx, query)
	}
	where, args := eventWhere(query)
	rows, err := store.db.QueryContext(ctx, `SELECT
		node_id,
		model_id,
		COALESCE(SUM(CASE WHEN event_type = 'request' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(CASE WHEN event_type = 'model_load' THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(CASE WHEN event_type = 'model_load' THEN duration_ms END), 0),
		COALESCE(MAX(CASE WHEN work_vram_max_mb > load_vram_after_mb THEN work_vram_max_mb ELSE load_vram_after_mb END), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_events `+where+`
		GROUP BY node_id, model_id
		ORDER BY COALESCE(SUM(CASE WHEN event_type = 'request' THEN 1 ELSE 0 END), 0) DESC, model_id
		LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ModelUsage
	for rows.Next() {
		var item ModelUsage
		if err := rows.Scan(&item.NodeID, &item.ModelID, &item.RequestCount, &item.TotalTokens, &item.ImageCount, &item.AudioSeconds, &item.LoadCount, &item.AverageLoadMS, &item.VRAMPeakMB, &item.VRAMPeakPct, &item.ModelVRAMMB); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) queryNodes(ctx context.Context, query Query) ([]NodeUsage, error) {
	if store.queryUsesRollups(query) {
		return store.queryRollupNodes(ctx, query)
	}
	where, args := eventWhere(query)
	rows, err := store.db.QueryContext(ctx, `SELECT
		node_id,
		COALESCE(SUM(CASE WHEN event_type = 'request' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(CASE WHEN event_type = 'model_load' THEN 1 ELSE 0 END), 0),
		COALESCE(AVG(CASE WHEN event_type = 'model_load' THEN duration_ms END), 0),
		COALESCE(MAX(CASE WHEN work_vram_max_mb > load_vram_after_mb THEN work_vram_max_mb ELSE load_vram_after_mb END), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_events `+where+`
		GROUP BY node_id
		ORDER BY node_id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []NodeUsage
	for rows.Next() {
		var item NodeUsage
		if err := rows.Scan(&item.NodeID, &item.RequestCount, &item.TotalTokens, &item.ImageCount, &item.AudioSeconds, &item.LoadCount, &item.AverageLoadMS, &item.VRAMPeakMB, &item.VRAMPeakPct, &item.ModelVRAMMB); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *Store) queryRecent(ctx context.Context, query Query) ([]RecentEvent, error) {
	where, args := eventWhere(query)
	rows, err := store.db.QueryContext(ctx, `SELECT
		node_id, model_id, section, backend_mode, event_type, route, config_filename, status_code, success,
		started_at, finished_at, duration_ms, request_bytes, response_bytes, input_tokens, output_tokens,
		total_tokens, tokens_per_second, image_count, image_width, image_height,
		image_type, audio_seconds, audio_tokens, load_vram_before_mb, load_vram_after_mb,
		load_vram_delta_mb, work_vram_start_mb, work_vram_max_mb, work_vram_end_mb,
		model_vram_estimate_mb, vram_total_mb, vram_peak_percent
		FROM analytics_events `+where+`
		ORDER BY finished_at DESC
		LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []RecentEvent
	for rows.Next() {
		var item RecentEvent
		var success int
		if err := rows.Scan(
			&item.NodeID,
			&item.ModelID,
			&item.Section,
			&item.BackendMode,
			&item.EventType,
			&item.Route,
			&item.ConfigFilename,
			&item.StatusCode,
			&success,
			&item.StartedAt,
			&item.FinishedAt,
			&item.DurationMS,
			&item.RequestBytes,
			&item.ResponseBytes,
			&item.InputTokens,
			&item.OutputTokens,
			&item.TotalTokens,
			&item.TokensPerSecond,
			&item.ImageCount,
			&item.ImageWidth,
			&item.ImageHeight,
			&item.ImageType,
			&item.AudioSeconds,
			&item.AudioTokens,
			&item.LoadVRAMBefore,
			&item.LoadVRAMAfter,
			&item.LoadVRAMDelta,
			&item.WorkVRAMStart,
			&item.WorkVRAMMax,
			&item.WorkVRAMEnd,
			&item.ModelVRAM,
			&item.VRAMTotal,
			&item.VRAMPeakPercent,
		); err != nil {
			return nil, err
		}
		item.Success = success == 1
		result = append(result, item)
	}
	return result, rows.Err()
}

func eventWhere(query Query) (string, []any) {
	clauses := []string{"finished_at >= ?", "finished_at <= ?"}
	args := []any{query.StartMS, query.EndMS}
	return filteredWhere(clauses, args, query)
}

func rollupWhere(query Query, granularity string) (string, []any) {
	clauses := []string{"period_kind = ?", "bucket_start >= ?", "bucket_start <= ?"}
	args := []any{granularity, query.StartMS, query.EndMS}
	return filteredWhere(clauses, args, query)
}

func filteredWhere(clauses []string, args []any, query Query) (string, []any) {
	if strings.TrimSpace(query.NodeID) != "" {
		clauses = append(clauses, "node_id = ?")
		args = append(args, strings.TrimSpace(query.NodeID))
	}
	if strings.TrimSpace(query.ModelID) != "" {
		clauses = append(clauses, "model_id = ?")
		args = append(args, strings.TrimSpace(query.ModelID))
	}
	if strings.TrimSpace(query.Section) != "" {
		clauses = append(clauses, "section = ?")
		args = append(args, strings.TrimSpace(query.Section))
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}
