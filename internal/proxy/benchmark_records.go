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

func (runner *benchmarkRunner) benchmarkRecord(ctx context.Context, query url.Values, nodeOnly bool) (routerbenchmark.Record, error) {
	identity := runner.deps.clusterIdentity()
	nodeID := strings.TrimSpace(query.Get("node_id"))
	modelID := strings.TrimSpace(query.Get("model_id"))
	if modelID == "" {
		return routerbenchmark.Record{}, fmt.Errorf("model_id is required")
	}
	if !nodeOnly && !runner.benchmarkTargetsLocal(nodeID) {
		nodeURL := runner.benchmarkNodeURL(nodeID)
		if nodeURL == "" {
			return routerbenchmark.Record{}, fmt.Errorf("node %q was not found", nodeID)
		}
		var record routerbenchmark.Record
		path := "/router/v1/node/benchmarks?model_id=" + url.QueryEscape(modelID)
		err := identity.client.JSON(ctx, http.MethodGet, nodeURL, path, nil, &record)
		return record, err
	}
	if runner.store == nil {
		return routerbenchmark.Record{}, fmt.Errorf("benchmark store is not configured")
	}
	record, ok, err := runner.store.Record(identity.nodeID, modelID)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if !ok {
		return routerbenchmark.Record{
			NodeID:   identity.nodeID,
			ModelID:  modelID,
			Sections: map[string]routerbenchmark.Summary{},
			History:  []routerbenchmark.Summary{},
		}, nil
	}
	return record, nil
}

func (runner *benchmarkRunner) benchmarkTargetsLocal(nodeID string) bool {
	nodeID = strings.TrimSpace(nodeID)
	return nodeID == "" || nodeID == runner.deps.clusterIdentity().nodeID || nodeID == "local"
}

func (runner *benchmarkRunner) benchmarkNodeURL(nodeID string) string {
	nodeURLByID := runner.deps.nodeURLByID()
	return nodeURLByID[strings.TrimSpace(nodeID)]
}

func (runner *benchmarkRunner) decorate(models []cluster.Model) []cluster.Model {
	if runner.store == nil {
		return models
	}
	keys := make([]routerbenchmark.ModelKey, len(models))
	for index := range models {
		keys[index] = routerbenchmark.ModelKey{NodeID: models[index].NodeID, ModelID: models[index].LocalID}
	}
	benchmarks := runner.store.ModelBenchmarks(keys)
	for index, key := range keys {
		if benchmark, ok := benchmarks[key]; ok {
			models[index].Benchmark = &benchmark
		}
	}
	return models
}
