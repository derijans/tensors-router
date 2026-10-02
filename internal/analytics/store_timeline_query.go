package analytics

import (
	"context"
	"time"
)

type timelineSectionRow struct {
	bucketStart     int64
	section         string
	requestCount    int64
	successCount    int64
	inputTokens     int64
	outputTokens    int64
	totalTokens     int64
	imageCount      int64
	embeddingCount  int64
	audioSeconds    float64
	loadCount       int64
	tokensPSSum     float64
	tokensPSSamples int64
	vramPeakMB      int64
	vramPeakPct     float64
	vramTotalMB     int64
	modelVRAMMB     int64
}

func (store *Store) queryTimeline(ctx context.Context, query Query, granularity string) ([]Timeline, error) {
	query.StartMS = bucketStart(time.UnixMilli(query.StartMS), granularity)
	where, args := rollupWhere(query, granularity)
	rows, err := store.reader.QueryContext(ctx, `SELECT
		bucket_start,
		section,
		COALESCE(SUM(request_count), 0),
		COALESCE(SUM(success_count), 0),
		COALESCE(SUM(input_tokens), 0),
		COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(total_tokens), 0),
		COALESCE(SUM(image_count), 0),
		COALESCE(SUM(embedding_count), 0),
		COALESCE(SUM(audio_seconds), 0),
		COALESCE(SUM(load_count), 0),
		COALESCE(SUM(tokens_per_second_sum), 0),
		COALESCE(SUM(tokens_per_second_count), 0),
		COALESCE(MAX(vram_peak_mb), 0),
		COALESCE(MAX(vram_peak_percent), 0),
		COALESCE(MAX(vram_total_mb), 0),
		COALESCE(MAX(model_vram_estimate_mb), 0)
		FROM analytics_rollups `+where+`
		GROUP BY bucket_start, section
		ORDER BY bucket_start, section`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var builder timelineBuilder
	for rows.Next() {
		var row timelineSectionRow
		if err := rows.Scan(
			&row.bucketStart,
			&row.section,
			&row.requestCount,
			&row.successCount,
			&row.inputTokens,
			&row.outputTokens,
			&row.totalTokens,
			&row.imageCount,
			&row.embeddingCount,
			&row.audioSeconds,
			&row.loadCount,
			&row.tokensPSSum,
			&row.tokensPSSamples,
			&row.vramPeakMB,
			&row.vramPeakPct,
			&row.vramTotalMB,
			&row.modelVRAMMB,
		); err != nil {
			return nil, err
		}
		builder.add(row)
	}
	return builder.timeline(), rows.Err()
}

type timelineBuilder struct {
	buckets          []Timeline
	tokensPSSums     []float64
	indexByBucketKey map[int64]int
}

func (builder *timelineBuilder) add(row timelineSectionRow) {
	index := builder.bucketIndex(row.bucketStart)
	bucket := &builder.buckets[index]
	bucket.RequestCount += row.requestCount
	bucket.FailureCount += row.requestCount - row.successCount
	bucket.InputTokens += row.inputTokens
	bucket.OutputTokens += row.outputTokens
	bucket.TotalTokens += row.totalTokens
	bucket.ImageCount += row.imageCount
	bucket.EmbeddingCount += row.embeddingCount
	bucket.AudioSeconds += row.audioSeconds
	bucket.LoadCount += row.loadCount
	bucket.TokensPSSamples += row.tokensPSSamples
	builder.tokensPSSums[index] += row.tokensPSSum
	bucket.VRAMPeakMB = maxInt64(bucket.VRAMPeakMB, row.vramPeakMB)
	bucket.VRAMPeakPct = maxFloat64(bucket.VRAMPeakPct, row.vramPeakPct)
	bucket.VRAMTotalMB = maxInt64(bucket.VRAMTotalMB, row.vramTotalMB)
	bucket.ModelVRAMMB = maxInt64(bucket.ModelVRAMMB, row.modelVRAMMB)
	if row.requestCount > 0 {
		bucket.Sections = append(bucket.Sections, TimelineSection{Section: row.section, RequestCount: row.requestCount})
	}
}

func (builder *timelineBuilder) bucketIndex(bucketStart int64) int {
	if builder.indexByBucketKey == nil {
		builder.indexByBucketKey = map[int64]int{}
	}
	if index, ok := builder.indexByBucketKey[bucketStart]; ok {
		return index
	}
	builder.buckets = append(builder.buckets, Timeline{BucketStart: bucketStart, Sections: []TimelineSection{}})
	builder.tokensPSSums = append(builder.tokensPSSums, 0)
	index := len(builder.buckets) - 1
	builder.indexByBucketKey[bucketStart] = index
	return index
}

func (builder *timelineBuilder) timeline() []Timeline {
	for index := range builder.buckets {
		bucket := &builder.buckets[index]
		if bucket.TokensPSSamples > 0 {
			bucket.AverageTokensPS = builder.tokensPSSums[index] / float64(bucket.TokensPSSamples)
		}
	}
	return builder.buckets
}
