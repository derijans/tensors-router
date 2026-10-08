package main

import (
	"context"
	"time"

	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/catalog"
	routercluster "tensors-router/internal/cluster"
	"tensors-router/internal/mcp"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/transportbody"
)

func (runtime *serveRuntime) startMCP() error {
	cfg := runtime.cfg
	reconciler, err := mcp.NewReconciler(mcp.Config{Enabled: cfg.MCP.Enabled, Directory: cfg.MCP.Directory, ConfigDir: cfg.Models.ConfigDir})
	if err != nil {
		return err
	}
	if err := reconciler.ReconcileAll(cfg.Backend.Mode); err != nil {
		return err
	}
	runtime.mcpReconciler = reconciler
	if cfg.MCP.Enabled {
		runtime.mcpGateway = mcp.NewGateway(mcp.GatewayConfig{
			MaxRequestBodyBytes:  cfg.Limits.MaxControlBodyMB * transportbody.MiB,
			MaxResponseBodyBytes: cfg.Limits.MaxControlBodyMB * transportbody.MiB,
		})
	}
	return nil
}

func (runtime *serveRuntime) discoverModels() ([]catalog.Model, error) {
	configDir := runtime.cfg.Models.ConfigDir
	logger := runtime.startupLogger
	started := time.Now()
	logger.Printf("model config discovery started directory=%q", configDir)
	modelCatalog, err := catalog.NewWithStore(configDir, runtime.cfg.Cluster.StoreDir)
	if err != nil {
		logger.Printf("model config discovery failed directory=%q elapsed=%s error=%v", configDir, time.Since(started), err)
		return nil, err
	}
	runtime.catalog = modelCatalog
	models, err := modelCatalog.List()
	if err != nil {
		logger.Printf("model config discovery failed directory=%q elapsed=%s error=%v", configDir, time.Since(started), err)
		return nil, err
	}
	logger.Printf("model config discovery completed directory=%q configs=%d elapsed=%s", configDir, len(models), time.Since(started))
	return models, nil
}

func (runtime *serveRuntime) openAssetIndex() error {
	assetIndex, err := modelassets.NewIndex(runtime.cfg.Cluster.StoreDir, runtime.cfg.Models.SharedDir)
	if err != nil {
		return err
	}
	assetIndex.SetHashWorkers(runtime.cfg.Models.HashWorkers)
	runtime.catalog.UseKnownFileHashes(assetIndex.CachedFileHash)
	runtime.assetIndex = assetIndex
	return nil
}

func (runtime *serveRuntime) registerLocalModels(ctx context.Context, discoveredModels []catalog.Model) error {
	cfg := runtime.cfg
	runtime.registry = routercluster.NewRegistry(cfg.Cluster.Role, cfg.Cluster.NodeID, cfg.Cluster.PublicURL)
	localSource := routercluster.SourceLocal
	if cfg.Cluster.Role == routercluster.RoleMaster {
		localSource = routercluster.SourceMaster
	}
	localClusterModels := routercluster.WithMCPAvailability(
		withModelBenchmarks(routercluster.LocalModelsWithBackendMode(discoveredModels, cfg.Cluster.NodeID, cfg.Cluster.PublicURL, localSource, cfg.Backend.Mode), runtime.stores.benchmarks),
		cfg.MCP.Enabled,
	)
	disabledModelIDs, err := runtime.stores.modelState.DisabledIDs(ctx)
	if err != nil {
		return err
	}
	for index := range localClusterModels {
		_, localClusterModels[index].Disabled = disabledModelIDs[localClusterModels[index].LocalID]
	}
	return runtime.registry.UpdateLocal(localClusterModels)
}

func withModelBenchmarks(models []routercluster.Model, store *routerbenchmark.Store) []routercluster.Model {
	if store == nil {
		return models
	}
	keys := make([]routerbenchmark.ModelKey, len(models))
	for index := range models {
		keys[index] = routerbenchmark.ModelKey{NodeID: models[index].NodeID, ModelID: models[index].LocalID}
	}
	benchmarks := store.ModelBenchmarks(keys)
	for index, key := range keys {
		if benchmark, ok := benchmarks[key]; ok {
			models[index].Benchmark = &benchmark
		}
	}
	return models
}
