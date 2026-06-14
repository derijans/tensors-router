package proxy

import (
	"context"
	"net/http"
	"strings"

	routerbenchmark "tensors-router/internal/benchmark"
)

const benchmarkPreviewLimit = 2048

func (service *Service) performBenchmarkRequest(ctx context.Context, path string, body string) (int, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, path, strings.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	recorder := newBenchmarkResponseWriter()
	service.ServeHTTP(recorder, request)
	return recorder.statusCode(), recorder.preview.String(), nil
}

type benchmarkResponseWriter struct {
	header  http.Header
	status  int
	preview strings.Builder
}

func newBenchmarkResponseWriter() *benchmarkResponseWriter {
	return &benchmarkResponseWriter{header: http.Header{}}
}

func (writer *benchmarkResponseWriter) Header() http.Header {
	return writer.header
}

func (writer *benchmarkResponseWriter) WriteHeader(status int) {
	if writer.status == 0 {
		writer.status = status
	}
}

func (writer *benchmarkResponseWriter) Write(body []byte) (int, error) {
	if writer.status == 0 {
		writer.status = http.StatusOK
	}
	remaining := benchmarkPreviewLimit - writer.preview.Len()
	if remaining > 0 {
		if len(body) < remaining {
			remaining = len(body)
		}
		writer.preview.Write(body[:remaining])
	}
	return len(body), nil
}

func (writer *benchmarkResponseWriter) statusCode() int {
	if writer.status == 0 {
		return http.StatusOK
	}
	return writer.status
}

func successMetric(name string, durationMS int64) routerbenchmark.Metric {
	return routerbenchmark.Metric{
		Name:       name,
		Status:     routerbenchmark.StatusSuccess,
		DurationMS: durationMS,
		Unit:       "ms",
	}
}

func successValueMetric(name string, value float64, unit string) routerbenchmark.Metric {
	return routerbenchmark.Metric{
		Name:   name,
		Status: routerbenchmark.StatusSuccess,
		Value:  value,
		Unit:   unit,
	}
}

func skippedMetric(name string, reason string) routerbenchmark.Metric {
	return routerbenchmark.Metric{
		Name:   name,
		Status: routerbenchmark.StatusSkipped,
		Error:  reason,
	}
}

func failedMetric(name string, message string, durationMS int64) routerbenchmark.Metric {
	return routerbenchmark.Metric{
		Name:       name,
		Status:     routerbenchmark.StatusFailed,
		DurationMS: durationMS,
		Unit:       "ms",
		Error:      message,
	}
}

func metricsStatus(metrics []routerbenchmark.Metric) string {
	success := 0
	failed := 0
	skipped := 0
	for _, metric := range metrics {
		switch metric.Status {
		case routerbenchmark.StatusSuccess:
			success++
		case routerbenchmark.StatusFailed:
			failed++
		default:
			skipped++
		}
	}
	switch {
	case failed == 0 && success > 0:
		return routerbenchmark.StatusSuccess
	case failed > 0 && success > 0:
		return routerbenchmark.StatusPartial
	case failed > 0:
		return routerbenchmark.StatusFailed
	case skipped > 0:
		return routerbenchmark.StatusSkipped
	default:
		return routerbenchmark.StatusSkipped
	}
}

func metricsError(metrics []routerbenchmark.Metric) string {
	for _, metric := range metrics {
		if metric.Status == routerbenchmark.StatusFailed && strings.TrimSpace(metric.Error) != "" {
			return metric.Error
		}
	}
	for _, metric := range metrics {
		if metric.Status == routerbenchmark.StatusSkipped && strings.TrimSpace(metric.Error) != "" {
			return metric.Error
		}
	}
	return ""
}
