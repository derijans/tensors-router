package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	routerbenchmark "tensors-router/internal/benchmark"
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
	var payload any
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return textBenchmarkStats{}
	}
	root, ok := payload.(map[string]any)
	if !ok {
		return textBenchmarkStats{}
	}
	stats := textBenchmarkStats{}
	stats.promptTokens = firstNestedNumber(root,
		[]string{"usage", "prompt_tokens"},
		[]string{"timings", "prompt_n"},
		[]string{"prompt_tokens"},
		[]string{"prompt_eval_count"},
	)
	stats.completionTokens = firstNestedNumber(root,
		[]string{"usage", "completion_tokens"},
		[]string{"timings", "predicted_n"},
		[]string{"completion_tokens"},
		[]string{"eval_count"},
	)
	stats.tokensPerSecond = firstNestedNumber(root,
		[]string{"timings", "predicted_per_second"},
		[]string{"predicted_per_second"},
		[]string{"tokens_per_second"},
	)
	stats.promptTokensPerSecond = firstNestedNumber(root,
		[]string{"timings", "prompt_per_second"},
		[]string{"prompt_per_second"},
	)
	if stats.tokensPerSecond <= 0 {
		durationSeconds := duration.Seconds()
		if stats.completionTokens > 0 && durationSeconds > 0 {
			stats.tokensPerSecond = stats.completionTokens / durationSeconds
		}
	}
	if stats.tokensPerSecond <= 0 {
		evalCount := firstNestedNumber(root, []string{"eval_count"})
		evalDuration := firstNestedNumber(root, []string{"eval_duration"})
		if evalCount > 0 && evalDuration > 0 {
			stats.tokensPerSecond = evalCount / (evalDuration / float64(time.Second))
		}
	}
	return stats
}

func firstNestedNumber(root map[string]any, paths ...[]string) float64 {
	for _, path := range paths {
		if value, ok := nestedNumber(root, path); ok {
			return value
		}
	}
	return 0
}

func nestedNumber(root map[string]any, path []string) (float64, bool) {
	var current any = root
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return 0, false
		}
		current, ok = object[key]
		if !ok {
			return 0, false
		}
	}
	return numberValue(current)
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case float64:
		return typed, true
	case int:
		return float64(typed), true
	default:
		return 0, false
	}
}
