package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/catalog"
)

func (runner *benchmarkRunner) runBenchmark(ctx context.Context, request routerbenchmark.RunRequest, nodeOnly bool) (routerbenchmark.Record, error) {
	request, err := normalizeBenchmarkRequest(request)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if request.ModelID == "" {
		return routerbenchmark.Record{}, fmt.Errorf("model_id is required")
	}
	if !nodeOnly && !runner.benchmarkTargetsLocal(request.NodeID) {
		return runner.runRemoteBenchmark(ctx, request)
	}
	return runner.runLocalBenchmark(ctx, request)
}

func (runner *benchmarkRunner) runRemoteBenchmark(ctx context.Context, request routerbenchmark.RunRequest) (routerbenchmark.Record, error) {
	identity := runner.deps.clusterIdentity()
	nodeURL := runner.benchmarkNodeURL(request.NodeID)
	if nodeURL == "" {
		return routerbenchmark.Record{}, fmt.Errorf("node %q was not found", request.NodeID)
	}
	var record routerbenchmark.Record
	err := identity.client.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/node/benchmarks/run", request, &record)
	if err != nil {
		return record, err
	}
	snapshot, err := identity.client.FetchSnapshot(ctx, nodeURL)
	if err != nil {
		runner.logger.Printf("benchmark remote snapshot refresh failed node=%q error=%v", request.NodeID, err)
		return record, nil
	}
	if identity.registry == nil {
		return record, nil
	}
	snapshot.NodeURL = nodeURL
	if err := identity.registry.UpdateNode(snapshot); err != nil {
		runner.logger.Printf("benchmark remote registry update failed node=%q error=%v", request.NodeID, err)
	}
	return record, nil
}

func (runner *benchmarkRunner) runLocalBenchmark(ctx context.Context, request routerbenchmark.RunRequest) (routerbenchmark.Record, error) {
	if runner.store == nil {
		return routerbenchmark.Record{}, fmt.Errorf("benchmark store is not configured")
	}
	model, ok, err := runner.deps.resolveCatalogModel(request.ModelID)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if !ok {
		return routerbenchmark.Record{}, fmt.Errorf("model %q was not found", request.ModelID)
	}
	enabled, err := runner.deps.localModelEnabled(ctx, model.ID)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if !enabled {
		return routerbenchmark.Record{}, fmt.Errorf("model %q is disabled", request.ModelID)
	}

	runContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), benchmarkTimeout(request.TimeoutSeconds))
	defer cancel()

	runner.mu.Lock()
	defer runner.mu.Unlock()

	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	sections := expandBenchmarkSections(request)
	summaries := make([]routerbenchmark.Summary, 0, len(sections))
	for _, section := range sections {
		summaries = append(summaries, runner.runBenchmarkSection(runContext, runID, request, model, section))
		if runContext.Err() != nil {
			break
		}
	}
	if len(summaries) == 0 {
		return routerbenchmark.Record{}, fmt.Errorf("no benchmark sections selected")
	}
	record, err := runner.store.SaveRun(runner.deps.clusterIdentity().nodeID, model.ID, request.Type, summaries, model.Options)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if err := runner.deps.refreshLocalRegistry(); err != nil {
		runner.logger.Printf("benchmark registry refresh failed: %v", err)
	}
	return record, nil
}

func (runner *benchmarkRunner) runBenchmarkSection(ctx context.Context, runID string, request routerbenchmark.RunRequest, model catalog.Model, section string) routerbenchmark.Summary {
	started := time.Now()
	summary := routerbenchmark.Summary{
		RunID:     runID,
		Type:      request.Type,
		Section:   section,
		Status:    routerbenchmark.StatusRunning,
		StartedAt: started.UnixMilli(),
	}
	metrics := runner.benchmarkMetrics(ctx, request, model, section)
	summary.Metrics = metrics
	finished := time.Now()
	summary.FinishedAt = finished.UnixMilli()
	summary.DurationMS = finished.Sub(started).Milliseconds()
	summary.Status = metricsStatus(metrics)
	summary.Error = metricsError(metrics)
	return summary
}

func (runner *benchmarkRunner) benchmarkMetrics(ctx context.Context, request routerbenchmark.RunRequest, model catalog.Model, section string) []routerbenchmark.Metric {
	switch section {
	case routerbenchmark.SectionRuntime:
		return []routerbenchmark.Metric{runner.runtimeBenchmarkMetric(ctx, model)}
	case routerbenchmark.SectionLLM:
		if !model.HasLLM {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no llm lane")}
		}
		return runner.textBenchmarkMetrics(ctx, "/v1/chat/completions", textBenchmarkBody(model.ID), request.Iterations)
	case routerbenchmark.SectionEmbed:
		if !model.HasEmbeddings && (model.BackendMode == BackendModeVLLM || !model.HasLLM) {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no embedding lane")}
		}
		return runner.requestBenchmarkMetrics(ctx, "/v1/embeddings", embeddingsBenchmarkBody(model.ID), request.Iterations)
	case routerbenchmark.SectionImage:
		if !model.HasImage {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no image lane")}
		}
		return runner.requestBenchmarkMetrics(ctx, "/v1/images/generations", imageBenchmarkBody(model.ImageID), request.Iterations)
	case routerbenchmark.SectionVoice:
		if !model.HasVoice {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no voice lane")}
		}
		if model.BackendMode == BackendModeVLLM {
			contentType, body, err := transcriptionBenchmarkBody(model.ID)
			if err != nil {
				return []routerbenchmark.Metric{failedMetric(section, err.Error(), 0)}
			}
			return runner.requestBenchmarkMetricsWithContentType(ctx, "/v1/audio/transcriptions", body, contentType, request.Iterations)
		}
		return runner.requestBenchmarkMetrics(ctx, "/v1/audio/speech", voiceBenchmarkBody(model.ID), request.Iterations)
	case routerbenchmark.SectionMusic:
		if !model.HasMusic {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no music lane")}
		}
		return runner.requestBenchmarkMetrics(ctx, "/api/extra/music/generate", musicBenchmarkBody(model.ID), request.Iterations)
	default:
		return []routerbenchmark.Metric{failedMetric(section, fmt.Sprintf("unknown benchmark section %q", section), 0)}
	}
}

func (runner *benchmarkRunner) runtimeBenchmarkMetric(ctx context.Context, model catalog.Model) routerbenchmark.Metric {
	started := time.Now()
	err := runner.deps.loadLocalModel(ctx, model.ID, model.ID)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		return failedMetric(routerbenchmark.MetricModelLoadMS, err.Error(), duration)
	}
	return successMetric(routerbenchmark.MetricModelLoadMS, duration)
}

func (runner *benchmarkRunner) requestBenchmarkMetrics(ctx context.Context, path string, body string, iterations int) []routerbenchmark.Metric {
	return runner.requestBenchmarkMetricsWithContentType(ctx, path, body, "application/json", iterations)
}

func (runner *benchmarkRunner) requestBenchmarkMetricsWithContentType(ctx context.Context, path string, body string, contentType string, iterations int) []routerbenchmark.Metric {
	var total time.Duration
	for index := 0; index < iterations; index++ {
		started := time.Now()
		status, preview, err := runner.performBenchmarkRequestWithContentType(ctx, path, body, contentType)
		duration := time.Since(started)
		total += duration
		if err != nil {
			return []routerbenchmark.Metric{failedMetric(routerbenchmark.MetricRequestMS, err.Error(), duration.Milliseconds())}
		}
		if status < 200 || status >= 300 {
			message := strings.TrimSpace(preview)
			if message == "" {
				message = fmt.Sprintf("request failed with status %d", status)
			}
			return []routerbenchmark.Metric{failedMetric(routerbenchmark.MetricRequestMS, message, duration.Milliseconds())}
		}
	}
	average := total / time.Duration(iterations)
	return []routerbenchmark.Metric{successMetric(routerbenchmark.MetricRequestMS, average.Milliseconds())}
}
