package proxy

import (
	"context"
	"net/http"
	"strings"
	"time"

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/inventory"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/proxy/clusterfan"
	"tensors-router/internal/recipes"
	"tensors-router/internal/siteapi"
)

func (service *Service) localNodeInventory(ctx context.Context, includeFiles bool) (siteapi.NodeInventory, error) {
	models, err := service.localClusterModels()
	if err != nil {
		return siteapi.NodeInventory{}, err
	}
	files := []inventory.FileRecord{}
	if includeFiles {
		started := time.Now()
		service.logger.Printf("model file inventory scan started roots=%d", len(service.assets.fileRoots))
		files, err = inventory.Scan(service.assets.fileRoots, models, service.nodeID)
		if err != nil {
			service.logger.Printf("model file inventory scan failed roots=%d elapsed=%s error=%v", len(service.assets.fileRoots), time.Since(started), err)
			return siteapi.NodeInventory{}, err
		}
		if service.assets.index != nil {
			for index := range files {
				files[index].SHA256, _ = service.assets.index.CachedFileHash(files[index].Path)
			}
		}
		service.logger.Printf("model file inventory scan completed roots=%d files=%d elapsed=%s", len(service.assets.fileRoots), len(files), time.Since(started))
	}
	return siteapi.NodeInventory{
		NodeID:       service.nodeID,
		NodeURL:      service.nodeURL,
		Source:       service.localSource(),
		Role:         service.clusterRole,
		BackendMode:  service.backendMode,
		Available:    true,
		Hardware:     service.hardware.Info(ctx),
		Models:       models,
		Files:        files,
		BuildVersion: buildinfo.Current().Version,
	}, nil
}

func inventoryFilesRequested(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_files")), "true")
}

func (service *Service) refreshLocalRegistryWithLogs() error {
	started := time.Now()
	service.logger.Printf("model config scan started")
	if err := service.refreshLocalRegistry(); err != nil {
		service.logger.Printf("model config scan failed elapsed=%s error=%v", time.Since(started), err)
		return err
	}
	service.logger.Printf("model config scan completed elapsed=%s", time.Since(started))
	return nil
}

func (service *Service) siteModels() []cluster.Model {
	if service.registry != nil {
		return service.benchmarks.decorate(service.registry.Models())
	}
	models, err := service.localClusterModels()
	if err != nil {
		return []cluster.Model{}
	}
	return models
}

func (service *Service) localClusterModels() ([]cluster.Model, error) {
	models, err := service.catalog.List()
	if err != nil {
		return nil, err
	}
	records := cluster.WithMCPAvailability(
		service.benchmarks.decorate(cluster.LocalModelsWithBackendMode(models, service.nodeID, service.nodeURL, service.localSource(), service.backendMode)),
		service.mcpGateway != nil,
	)
	if service.modelStateStore != nil {
		disabled, err := service.modelStateStore.DisabledIDs(context.Background())
		if err != nil {
			return nil, err
		}
		for index := range records {
			_, records[index].Disabled = disabled[records[index].LocalID]
		}
	}
	if service.assets.index == nil {
		return records, nil
	}
	states, err := service.assets.index.LatestResolutionStates()
	if err != nil {
		return nil, err
	}
	for index := range records {
		job, found := states[records[index].LocalID]
		if !found || records[index].AssetState == "ready" {
			continue
		}
		if job.State == modelassets.JobQueued || job.State == modelassets.JobResolving {
			records[index].AssetState = "resolving"
		}
		if job.State == modelassets.JobFailed {
			records[index].AssetState = "failed"
			records[index].AssetFailure = "model asset unavailable"
		}
	}
	return records, nil
}

func (service *Service) localSource() string {
	if service.clusterRole == cluster.RoleMaster {
		return cluster.SourceMaster
	}
	return cluster.SourceLocal
}

func (service *Service) refreshLocalRegistry() error {
	defer service.webUI.invalidate()
	if refresher, ok := service.catalog.(interface{ Refresh() error }); ok {
		if err := refresher.Refresh(); err != nil {
			return err
		}
	}
	if service.registry == nil {
		return nil
	}
	models, err := service.localClusterModels()
	if err != nil {
		return err
	}
	return service.registry.UpdateLocal(models)
}

func (service *Service) remoteInventoryURLs() []string {
	values := append([]string{}, service.slaveURLs...)
	if service.registry != nil {
		for nodeID, nodeURL := range service.registry.NodeURLsByID() {
			if nodeID == service.nodeID || strings.TrimSpace(nodeURL) == "" {
				continue
			}
			values = append(values, nodeURL)
		}
	}
	return clusterfan.UniqueTargets(values)
}

const inventoryWithFilesTimeout = 30 * time.Second

func (service *Service) siteInventory(ctx context.Context, includeFiles bool) (siteapi.InventoryResponse, error) {
	if includeFiles {
		if err := service.refreshLocalRegistryWithLogs(); err != nil {
			return siteapi.InventoryResponse{}, err
		}
	}
	localNode, err := service.localNodeInventory(ctx, includeFiles)
	if err != nil {
		return siteapi.InventoryResponse{}, err
	}
	nodes := []siteapi.NodeInventory{localNode}
	if service.clusterRole == cluster.RoleMaster {
		nodes = append(nodes, service.remoteNodeInventories(ctx, includeFiles)...)
	}
	models := withReportedAssetStates(service.siteModels(), nodes)
	recipesList, err := service.storedRecipes()
	if err != nil {
		return siteapi.InventoryResponse{}, err
	}
	return siteapi.InventoryResponse{
		Role:            service.clusterRole,
		NodeID:          service.nodeID,
		NodeURL:         service.nodeURL,
		Nodes:           nodes,
		Models:          models,
		Recipes:         recipesList,
		OptionCatalog:   cook.OptionCatalog(),
		ObservedOptions: observedOptions(nodes, models),
	}, nil
}

func (service *Service) remoteNodeInventories(ctx context.Context, includeFiles bool) []siteapi.NodeInventory {
	timeout := clusterfan.Timeout
	path := "/router/v1/node/site/inventory"
	if includeFiles {
		timeout = inventoryWithFilesTimeout
		path += "?include_files=true"
	}
	results := clusterfan.NodesWithin(ctx, service.remoteInventoryURLs(), timeout, clusterfan.Limit, func(nodeContext context.Context, nodeURL string) (siteapi.NodeInventory, error) {
		remoteNode := siteapi.NodeInventory{
			NodeID:    service.nodeIDForURL(nodeURL),
			NodeURL:   nodeURL,
			Source:    cluster.SourceSlave,
			Role:      cluster.RoleSlave,
			Available: false,
		}
		err := service.clusterClient.JSON(nodeContext, http.MethodGet, nodeURL, path, nil, &remoteNode)
		return remoteNode, err
	})
	nodes := make([]siteapi.NodeInventory, 0, len(results))
	for _, result := range results {
		remoteNode := result.Value
		if result.Err != nil {
			remoteNode.Error = result.Err.Error()
			service.logger.Printf("node inventory failed node=%q include_files=%t error=%v", result.Target, includeFiles, result.Err)
		}
		nodes = append(nodes, remoteNode)
	}
	return nodes
}

func withReportedAssetStates(models []cluster.Model, nodes []siteapi.NodeInventory) []cluster.Model {
	assetStates := map[string]cluster.Model{}
	for _, node := range nodes {
		for _, model := range node.Models {
			assetStates[node.NodeID+"\x00"+model.LocalID] = model
		}
	}
	for index := range models {
		if current, found := assetStates[models[index].NodeID+"\x00"+models[index].LocalID]; found {
			models[index].AssetState = current.AssetState
			models[index].UnresolvedFields = current.UnresolvedFields
			models[index].AssetFailure = current.AssetFailure
		}
	}
	return models
}

func (service *Service) storedRecipes() ([]recipes.Recipe, error) {
	if service.recipeStore == nil {
		return []recipes.Recipe{}, nil
	}
	return service.recipeStore.List()
}
