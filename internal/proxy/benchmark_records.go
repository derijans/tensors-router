package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/cluster"
)

func (service *Service) benchmarkRecord(ctx context.Context, query url.Values, nodeOnly bool) (routerbenchmark.Record, error) {
	nodeID := strings.TrimSpace(query.Get("node_id"))
	modelID := strings.TrimSpace(query.Get("model_id"))
	if modelID == "" {
		return routerbenchmark.Record{}, fmt.Errorf("model_id is required")
	}
	if !nodeOnly && !service.benchmarkTargetsLocal(nodeID) {
		nodeURL := service.benchmarkNodeURL(nodeID)
		if nodeURL == "" {
			return routerbenchmark.Record{}, fmt.Errorf("node %q was not found", nodeID)
		}
		var record routerbenchmark.Record
		path := "/router/v1/node/benchmarks?model_id=" + url.QueryEscape(modelID)
		err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, path, nil, &record)
		return record, err
	}
	if service.benchmarkStore == nil {
		return routerbenchmark.Record{}, fmt.Errorf("benchmark store is not configured")
	}
	record, ok, err := service.benchmarkStore.Record(service.nodeID, modelID)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if !ok {
		return routerbenchmark.Record{
			NodeID:   service.nodeID,
			ModelID:  modelID,
			Sections: map[string]routerbenchmark.Summary{},
			History:  []routerbenchmark.Summary{},
		}, nil
	}
	return record, nil
}

func (service *Service) benchmarkTargetsLocal(nodeID string) bool {
	nodeID = strings.TrimSpace(nodeID)
	return nodeID == "" || nodeID == service.nodeID || nodeID == "local"
}

func (service *Service) benchmarkNodeURL(nodeID string) string {
	nodeURLByID := service.nodeURLByID()
	return nodeURLByID[strings.TrimSpace(nodeID)]
}

func (service *Service) withBenchmarks(models []cluster.Model) []cluster.Model {
	if service.benchmarkStore == nil {
		return models
	}
	for index := range models {
		benchmark, ok, err := service.benchmarkStore.ModelBenchmark(models[index].NodeID, models[index].LocalID)
		if err != nil {
			service.logger.Printf("benchmark model lookup failed node=%q model=%q error=%v", models[index].NodeID, models[index].LocalID, err)
			continue
		}
		if ok {
			models[index].Benchmark = &benchmark
		}
	}
	return models
}
