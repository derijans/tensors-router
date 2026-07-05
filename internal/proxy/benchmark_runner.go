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

func (service *Service) runBenchmark(ctx context.Context, request routerbenchmark.RunRequest, nodeOnly bool) (routerbenchmark.Record, error) {
	request, err := normalizeBenchmarkRequest(request)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if request.ModelID == "" {
		return routerbenchmark.Record{}, fmt.Errorf("model_id is required")
	}
	if !nodeOnly && !service.benchmarkTargetsLocal(request.NodeID) {
		return service.runRemoteBenchmark(ctx, request)
	}
	return service.runLocalBenchmark(ctx, request)
}

func (service *Service) runRemoteBenchmark(ctx context.Context, request routerbenchmark.RunRequest) (routerbenchmark.Record, error) {
	nodeURL := service.benchmarkNodeURL(request.NodeID)
	if nodeURL == "" {
		return routerbenchmark.Record{}, fmt.Errorf("node %q was not found", request.NodeID)
	}
	var record routerbenchmark.Record
	err := service.clusterClient.JSON(ctx, http.MethodPost, nodeURL, "/router/v1/node/benchmarks/run", request, &record)
	if err != nil {
		return record, err
	}
	snapshot, err := service.clusterClient.FetchSnapshot(ctx, nodeURL)
	if err != nil {
		service.logger.Printf("benchmark remote snapshot refresh failed node=%q error=%v", request.NodeID, err)
		return record, nil
	}
	if service.registry == nil {
		return record, nil
	}
	snapshot.NodeURL = nodeURL
	if err := service.registry.UpdateNode(snapshot); err != nil {
		service.logger.Printf("benchmark remote registry update failed node=%q error=%v", request.NodeID, err)
	}
	return record, nil
}

func (service *Service) runLocalBenchmark(ctx context.Context, request routerbenchmark.RunRequest) (routerbenchmark.Record, error) {
	if service.benchmarkStore == nil {
		return routerbenchmark.Record{}, fmt.Errorf("benchmark store is not configured")
	}
	model, ok, err := service.catalog.Resolve(request.ModelID)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if !ok {
		return routerbenchmark.Record{}, fmt.Errorf("model %q was not found", request.ModelID)
	}

	runContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), benchmarkTimeout(request.TimeoutSeconds))
	defer cancel()

	service.benchmarkMu.Lock()
	defer service.benchmarkMu.Unlock()

	runID := strconv.FormatInt(time.Now().UnixNano(), 36)
	sections := expandBenchmarkSections(request)
	summaries := make([]routerbenchmark.Summary, 0, len(sections))
	for _, section := range sections {
		summaries = append(summaries, service.runBenchmarkSection(runContext, runID, request, model, section))
		if runContext.Err() != nil {
			break
		}
	}
	if len(summaries) == 0 {
		return routerbenchmark.Record{}, fmt.Errorf("no benchmark sections selected")
	}
	record, err := service.benchmarkStore.SaveRun(service.nodeID, model.ID, request.Type, summaries, model.Options)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if err := service.refreshLocalRegistry(); err != nil {
		service.logger.Printf("benchmark registry refresh failed: %v", err)
	}
	return record, nil
}

func (service *Service) runBenchmarkSection(ctx context.Context, runID string, request routerbenchmark.RunRequest, model catalog.Model, section string) routerbenchmark.Summary {
	started := time.Now()
	summary := routerbenchmark.Summary{
		RunID:     runID,
		Type:      request.Type,
		Section:   section,
		Status:    routerbenchmark.StatusRunning,
		StartedAt: started.UnixMilli(),
	}
	metrics := service.benchmarkMetrics(ctx, request, model, section)
	summary.Metrics = metrics
	finished := time.Now()
	summary.FinishedAt = finished.UnixMilli()
	summary.DurationMS = finished.Sub(started).Milliseconds()
	summary.Status = metricsStatus(metrics)
	summary.Error = metricsError(metrics)
	return summary
}

func (service *Service) benchmarkMetrics(ctx context.Context, request routerbenchmark.RunRequest, model catalog.Model, section string) []routerbenchmark.Metric {
	switch section {
	case routerbenchmark.SectionRuntime:
		return []routerbenchmark.Metric{service.runtimeBenchmarkMetric(ctx, model)}
	case routerbenchmark.SectionLLM:
		if !model.HasLLM {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no llm lane")}
		}
		return service.textBenchmarkMetrics(ctx, "/v1/chat/completions", textBenchmarkBody(model.ID), request.Iterations)
	case routerbenchmark.SectionEmbed:
		if !model.HasEmbeddings && !model.HasLLM {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no embedding lane")}
		}
		return service.requestBenchmarkMetrics(ctx, "/v1/embeddings", embeddingsBenchmarkBody(model.ID), request.Iterations)
	case routerbenchmark.SectionImage:
		if !model.HasImage {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no image lane")}
		}
		return service.requestBenchmarkMetrics(ctx, "/v1/images/generations", imageBenchmarkBody(model.ImageID), request.Iterations)
	case routerbenchmark.SectionVoice:
		if !model.HasVoice {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no voice lane")}
		}
		return service.requestBenchmarkMetrics(ctx, "/v1/audio/speech", voiceBenchmarkBody(model.ID), request.Iterations)
	case routerbenchmark.SectionMusic:
		if !model.HasMusic {
			return []routerbenchmark.Metric{skippedMetric(section, "model has no music lane")}
		}
		return service.requestBenchmarkMetrics(ctx, "/api/extra/music/generate", musicBenchmarkBody(model.ID), request.Iterations)
	default:
		return []routerbenchmark.Metric{failedMetric(section, fmt.Sprintf("unknown benchmark section %q", section), 0)}
	}
}

func (service *Service) runtimeBenchmarkMetric(ctx context.Context, model catalog.Model) routerbenchmark.Metric {
	started := time.Now()
	err := service.loadLocalModel(ctx, model.ID, model.ID, "")
	duration := time.Since(started).Milliseconds()
	if err != nil {
		return failedMetric(routerbenchmark.MetricModelLoadMS, err.Error(), duration)
	}
	return successMetric(routerbenchmark.MetricModelLoadMS, duration)
}

func (service *Service) requestBenchmarkMetrics(ctx context.Context, path string, body string, iterations int) []routerbenchmark.Metric {
	var total time.Duration
	for index := 0; index < iterations; index++ {
		started := time.Now()
		status, preview, err := service.performBenchmarkRequest(ctx, path, body)
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
