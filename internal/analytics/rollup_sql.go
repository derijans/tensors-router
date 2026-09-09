package analytics

import "strings"

const (
	mergeBySum     = "sum"
	mergeByMaximum = "maximum"
)

type rollupMergeRule struct {
	column string
	merge  string
}

func rollupKeyColumns() []string {
	return []string{"period_kind", "bucket_start", "node_id", "model_id", "section", "backend_mode", "route"}
}

func rollupMergeRules() []rollupMergeRule {
	return []rollupMergeRule{
		{"request_count", mergeBySum},
		{"success_count", mergeBySum},
		{"duration_ms_total", mergeBySum},
		{"input_tokens", mergeBySum},
		{"output_tokens", mergeBySum},
		{"total_tokens", mergeBySum},
		{"tokens_per_second_sum", mergeBySum},
		{"tokens_per_second_count", mergeBySum},
		{"image_count", mergeBySum},
		{"audio_seconds", mergeBySum},
		{"audio_tokens", mergeBySum},
		{"load_count", mergeBySum},
		{"load_duration_ms_total", mergeBySum},
		{"vram_peak_mb", mergeByMaximum},
		{"vram_peak_percent", mergeByMaximum},
		{"vram_total_mb", mergeByMaximum},
		{"model_vram_estimate_mb", mergeByMaximum},
	}
}

func rollupColumns() []string {
	columns := rollupKeyColumns()
	for _, rule := range rollupMergeRules() {
		columns = append(columns, rule.column)
	}
	return columns
}

func rollupInsertSQL() string {
	columns := rollupColumns()
	placeholders := strings.TrimSuffix(strings.Repeat("?, ", len(columns)), ", ")
	return `INSERT INTO analytics_rollups (` + strings.Join(columns, ", ") + `)
	VALUES (` + placeholders + `)
	` + rollupMergeClause(columns)
}

// Two rollup rows sharing a bucket describe the same traffic counted twice, so
// the counters add and the peaks keep whichever reading was higher.
func rollupMergeClause(columns []string) string {
	present := make(map[string]bool, len(columns))
	for _, column := range columns {
		present[column] = true
	}
	conflict := `ON CONFLICT(` + strings.Join(rollupKeyColumns(), ", ") + `)`
	assignments := make([]string, 0, len(columns))
	for _, rule := range rollupMergeRules() {
		if !present[rule.column] {
			continue
		}
		switch rule.merge {
		case mergeByMaximum:
			assignments = append(assignments, rule.column+" = MAX(analytics_rollups."+rule.column+", excluded."+rule.column+")")
		default:
			assignments = append(assignments, rule.column+" = analytics_rollups."+rule.column+" + excluded."+rule.column)
		}
	}
	if len(assignments) == 0 {
		return conflict + " DO NOTHING"
	}
	return conflict + " DO UPDATE SET " + strings.Join(assignments, ", ")
}
