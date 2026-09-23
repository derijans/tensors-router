package proxy

import (
	"context"
	"log"
	"net/http"
	"sync"

	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/catalog"
	"tensors-router/internal/proxy/routing"
)

type benchmarkDeps interface {
	http.Handler
	siteControlAllowed() bool
	rejectModelLoadWhileDraining(w http.ResponseWriter) bool
	resolveCatalogModel(modelID string) (catalog.Model, bool, error)
	localModelEnabled(ctx context.Context, localID string) (bool, error)
	loadLocalModel(ctx context.Context, publicID string, localID string) error
	refreshLocalRegistry() error
	nodeURLByID() map[string]string
	clusterIdentity() clusterIdentity
}

type benchmarkRunner struct {
	deps   benchmarkDeps
	store  *routerbenchmark.Store
	mu     sync.Mutex
	logger *log.Logger
}

func newBenchmarkRunner(deps benchmarkDeps, store *routerbenchmark.Store, logger *log.Logger) *benchmarkRunner {
	return &benchmarkRunner{deps: deps, store: store, logger: logger}
}

func (runner *benchmarkRunner) routes() []routing.Route {
	return append([]routing.Route{
		routing.Exact(http.MethodGet, "/router/v1/benchmarks", runner.handleBenchmarks),
		routing.Exact(http.MethodPost, "/router/v1/benchmarks/run", runner.handleBenchmarkRun),
	}, routing.ClusterOnly([]routing.Route{
		routing.Exact(http.MethodGet, "/router/v1/node/benchmarks", runner.handleNodeBenchmarks),
		routing.Exact(http.MethodPost, "/router/v1/node/benchmarks/run", runner.handleNodeBenchmarkRun),
	})...)
}
