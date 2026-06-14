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
