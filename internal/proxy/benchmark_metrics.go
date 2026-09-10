package proxy

import (
	"context"
	"fmt"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/jsonpath"
)

type textBenchmarkStats struct {
	promptTokens          float64
	completionTokens      float64
	tokensPerSecond       float64
	promptTokensPerSecond float64
}

func (service *Service) textBenchmarkMetrics(ctx context.Context, path string, body string, iterations int) []routerbenchmark.Metric {
	var total time.Duration
	stats := textBenchmarkStats{}
	tokensPerSecondSamples := 0
	promptTokensPerSecondSamples := 0
	for index := 0; index < iterations; index++ {
		started := time.Now()
		status, responseBody, err := service.performBenchmarkRequest(ctx, path, body)
		duration := time.Since(started)
		total += duration
		if err != nil {
			return []routerbenchmark.Metric{failedMetric(routerbenchmark.MetricRequestMS, err.Error(), duration.Milliseconds())}
		}
		if status < 200 || status >= 300 {
			message := strings.TrimSpace(responseBody)
			if message == "" {
				message = fmt.Sprintf("request failed with status %d", status)
			}
			return []routerbenchmark.Metric{failedMetric(routerbenchmark.MetricRequestMS, message, duration.Milliseconds())}
		}
		current := extractTextBenchmarkStats(responseBody, duration)
		stats.promptTokens += current.promptTokens
		stats.completionTokens += current.completionTokens
		if current.tokensPerSecond > 0 {
			stats.tokensPerSecond += current.tokensPerSecond
			tokensPerSecondSamples++
		}
		if current.promptTokensPerSecond > 0 {
			stats.promptTokensPerSecond += current.promptTokensPerSecond
			promptTokensPerSecondSamples++
		}
	}
	average := total / time.Duration(iterations)
	metrics := []routerbenchmark.Metric{successMetric(routerbenchmark.MetricRequestMS, average.Milliseconds())}
	if stats.promptTokens > 0 {
		metrics = append(metrics, successValueMetric(routerbenchmark.MetricPromptTokens, stats.promptTokens/float64(iterations), "tokens"))
	}
	if stats.completionTokens > 0 {
		metrics = append(metrics, successValueMetric(routerbenchmark.MetricCompletionTokens, stats.completionTokens/float64(iterations), "tokens"))
	}
	if tokensPerSecondSamples > 0 {
		metrics = append(metrics, successValueMetric(routerbenchmark.MetricTokensPerSecond, stats.tokensPerSecond/float64(tokensPerSecondSamples), "tokens/s"))
	}
	if promptTokensPerSecondSamples > 0 {
		metrics = append(metrics, successValueMetric(routerbenchmark.MetricPromptTokensPerSecond, stats.promptTokensPerSecond/float64(promptTokensPerSecondSamples), "tokens/s"))
	}
	return metrics
}

func extractTextBenchmarkStats(body string, duration time.Duration) textBenchmarkStats {
	event := routeranalytics.Event{DurationMS: duration.Milliseconds()}
	routeranalytics.ApplyResponse(&event, "application/json", []byte(body))
	routeranalytics.DeriveTotals(&event)
	stats := textBenchmarkStats{
		promptTokens:     float64(event.InputTokens),
		completionTokens: float64(event.OutputTokens),
		tokensPerSecond:  event.TokensPerSecond,
	}
	root, ok := jsonpath.DecodeObject([]byte(body))
	if !ok {
		return stats
	}
	stats.promptTokensPerSecond = jsonpath.FirstNumber(root,
		[]string{"timings", "prompt_per_second"},
		[]string{"prompt_per_second"},
	)
	return stats
}
