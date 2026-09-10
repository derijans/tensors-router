package proxy

import (
	"math"
	"testing"
	"time"
)

func TestExtractTextBenchmarkStatsReadsOpenAIUsage(t *testing.T) {
	stats := extractTextBenchmarkStats(`{"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`, 2*time.Second)
	assertStat(t, "prompt tokens", stats.promptTokens, 10)
	assertStat(t, "completion tokens", stats.completionTokens, 20)
	assertStat(t, "tokens per second", stats.tokensPerSecond, 10)
}

func TestExtractTextBenchmarkStatsPrefersLlamaTimings(t *testing.T) {
	stats := extractTextBenchmarkStats(`{"timings":{"prompt_n":5,"predicted_n":7,"predicted_per_second":15.5,"prompt_per_second":90.25}}`, 10*time.Second)
	assertStat(t, "prompt tokens", stats.promptTokens, 5)
	assertStat(t, "completion tokens", stats.completionTokens, 7)
	assertStat(t, "tokens per second", stats.tokensPerSecond, 15.5)
	assertStat(t, "prompt tokens per second", stats.promptTokensPerSecond, 90.25)
}

func TestExtractTextBenchmarkStatsReadsOllamaCounts(t *testing.T) {
	stats := extractTextBenchmarkStats(`{"prompt_eval_count":3,"eval_count":4,"eval_duration":2000000000}`, 5*time.Second)
	assertStat(t, "prompt tokens", stats.promptTokens, 3)
	assertStat(t, "completion tokens", stats.completionTokens, 4)
	assertStat(t, "tokens per second", stats.tokensPerSecond, 2)
}

func TestExtractTextBenchmarkStatsIgnoresPlaceholderOllamaDurations(t *testing.T) {
	stats := extractTextBenchmarkStats(`{"prompt_eval_count":3,"eval_count":4,"eval_duration":1}`, 2*time.Second)
	assertStat(t, "tokens per second", stats.tokensPerSecond, 2)
}

func TestExtractTextBenchmarkStatsReadsKoboldNativeCounts(t *testing.T) {
	stats := extractTextBenchmarkStats(`{"results":[{"text":"hi","prompt_tokens":11,"completion_tokens":153}]}`, 6*time.Second)
	assertStat(t, "prompt tokens", stats.promptTokens, 11)
	assertStat(t, "completion tokens", stats.completionTokens, 153)
	assertStat(t, "tokens per second", stats.tokensPerSecond, 153.0/6.0)
}

func TestExtractTextBenchmarkStatsFallsBackToWallClock(t *testing.T) {
	stats := extractTextBenchmarkStats(`{"usage":{"completion_tokens":8}}`, 4*time.Second)
	assertStat(t, "tokens per second", stats.tokensPerSecond, 2)
}

func TestExtractTextBenchmarkStatsToleratesNonJSON(t *testing.T) {
	stats := extractTextBenchmarkStats("not json", time.Second)
	if stats != (textBenchmarkStats{}) {
		t.Fatalf("unexpected stats %#v", stats)
	}
}

func assertStat(t *testing.T, name string, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}
