package proxy

import (
	"context"
	"sort"
	"strings"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

type selectorlessEmbeddingTarget struct {
	publicID       string
	localID        string
	configFilename string
	backendMode    string
	remote         bool
	nodeURL        string
	release        func()
	catalogModel   catalog.Model
	clusterModel   cluster.Model
	clusterRoute   cluster.Route
}

func (service *Service) acquireSelectorlessEmbeddingTarget(rPath string, ctx context.Context) (selectorlessEmbeddingTarget, bool) {
	if service.registry != nil {
		return service.acquireSelectorlessRegistryEmbeddingTarget(rPath, ctx)
	}
	return service.selectSelectorlessCatalogEmbeddingTarget(rPath, ctx)
}

func (service *Service) acquireSelectorlessRegistryEmbeddingTarget(path string, ctx context.Context) (selectorlessEmbeddingTarget, bool) {
	candidates := selectorlessRegistryEmbeddingCandidates(service.modelsWithRuntimeState(ctx, service.registry.Models()), path)
	return service.acquireSelectorlessRegistryEmbeddingCandidates(path, ctx, candidates)
}

func (service *Service) acquireSelectorlessRegistryEmbeddingCandidates(path string, ctx context.Context, candidates []cluster.Model) (selectorlessEmbeddingTarget, bool) {
	start := service.nextEmbeddingCandidate(len(candidates))
	for offset := range len(candidates) {
		model := candidates[(start+offset)%len(candidates)]
		mode, err := service.clusterModelBackendMode(model)
		if err != nil {
			continue
		}
		readiness := embeddingReadinessForModel(path, mode, model.VLLMTask)
		localHealthy := true
		if model.Source != cluster.SourceSlave {
			localHealthy = service.localBackendAvailableForRoute(ctx, mode, readiness)
		}
		route, release, ok := service.registry.AcquireSpecificEmbedding(model.NodeID, model.Filename, model.LocalID, localHealthy)
		if !ok {
			continue
		}
		mode, err = service.clusterRouteBackendMode(route, model)
		if err != nil {
			release()
			continue
		}
		localID := route.LocalID
		if mode == BackendModeVLLM {
			localID = vllmRequestModelID(model.PublicID, route.LocalID, model.ServedNames)
		}
		return selectorlessEmbeddingTarget{
			publicID:       model.PublicID,
			localID:        localID,
			configFilename: route.Filename,
			backendMode:    mode,
			remote:         route.Remote,
			nodeURL:        route.NodeURL,
			release:        release,
			clusterModel:   model,
			clusterRoute:   route,
		}, true
	}
	return selectorlessEmbeddingTarget{}, false
}

func (service *Service) selectSelectorlessCatalogEmbeddingTarget(path string, ctx context.Context) (selectorlessEmbeddingTarget, bool) {
	if service.catalog == nil {
		return selectorlessEmbeddingTarget{}, false
	}
	models, err := service.catalog.List()
	if err != nil {
		return selectorlessEmbeddingTarget{}, false
	}
	candidates := make([]catalog.Model, 0, len(models))
	for _, model := range models {
		if !model.HasEmbeddings || !modelSupportsOpenAIPath(model, path) {
			continue
		}
		enabled, enabledErr := service.localModelEnabled(ctx, model.ID)
		if enabledErr != nil || !enabled {
			continue
		}
		mode, modeErr := service.catalogModelBackendMode(model)
		if modeErr != nil || !service.localEmbeddingModelLoaded(ctx, mode, model.Filename) {
			continue
		}
		candidates = append(candidates, model)
	}
	sort.Slice(candidates, func(left, right int) bool {
		return catalogEmbeddingCandidateKey(candidates[left]) < catalogEmbeddingCandidateKey(candidates[right])
	})
	if len(candidates) == 0 {
		return selectorlessEmbeddingTarget{}, false
	}
	model := candidates[service.nextEmbeddingCandidate(len(candidates))]
	mode, err := service.catalogModelBackendMode(model)
	if err != nil || !service.localEmbeddingModelLoaded(ctx, mode, model.Filename) {
		return selectorlessEmbeddingTarget{}, false
	}
	localID := model.ID
	if mode == BackendModeVLLM {
		localID = vllmRequestModelID(model.ID, model.ID, model.ServedNames)
	}
	return selectorlessEmbeddingTarget{
		publicID:       model.ID,
		localID:        localID,
		configFilename: model.Filename,
		backendMode:    mode,
		release:        func() {},
		catalogModel:   model,
	}, true
}

func selectorlessRegistryEmbeddingCandidates(models []cluster.Model, path string) []cluster.Model {
	byRoute := make(map[string]cluster.Model)
	for _, model := range models {
		if model.Disabled || !model.Available || !model.EmbeddingsLoaded || !model.HasEmbeddings || !registryModelSupportsOpenAIPath(model, path) {
			continue
		}
		key := clusterEmbeddingCandidateKey(model)
		existing, found := byRoute[key]
		if !found || embeddingPublicIDPreferred(model, existing) {
			byRoute[key] = model
		}
	}
	candidates := make([]cluster.Model, 0, len(byRoute))
	for _, model := range byRoute {
		candidates = append(candidates, model)
	}
	sort.Slice(candidates, func(left, right int) bool {
		return clusterEmbeddingCandidateKey(candidates[left]) < clusterEmbeddingCandidateKey(candidates[right])
	})
	return candidates
}

func embeddingPublicIDPreferred(candidate cluster.Model, existing cluster.Model) bool {
	candidatePrimary := candidate.PublicID == candidate.LocalID
	existingPrimary := existing.PublicID == existing.LocalID
	if candidatePrimary != existingPrimary {
		return candidatePrimary
	}
	return candidate.PublicID < existing.PublicID
}

func clusterEmbeddingCandidateKey(model cluster.Model) string {
	return strings.Join([]string{model.NodeID, model.BackendMode, model.Filename, model.LocalID}, "\x00")
}

func catalogEmbeddingCandidateKey(model catalog.Model) string {
	return strings.Join([]string{model.BackendMode, model.Filename, model.ID}, "\x00")
}

func (service *Service) nextEmbeddingCandidate(count int) int {
	if count == 0 {
		return 0
	}
	service.embeddingRoundRobinMu.Lock()
	index := int(service.embeddingRoundRobinNext % uint64(count))
	service.embeddingRoundRobinNext++
	service.embeddingRoundRobinMu.Unlock()
	return index
}

func (service *Service) localEmbeddingModelLoaded(ctx context.Context, mode string, filename string) bool {
	runtime, err := service.runtimeForBackendMode(mode, readinessEmbeddings)
	if err != nil || runtime == nil || runtime.backend == nil || !runtime.backend.Healthy(ctx) {
		return false
	}
	runtime.state.mu.Lock()
	loaded := !runtime.state.switching && runtime.state.filename == filename
	runtime.state.mu.Unlock()
	return loaded
}

func embeddingReadinessForModel(path string, mode string, task string) backendReadiness {
	if mode == BackendModeVLLM {
		return vllmReadinessForTask(path, task)
	}
	return readinessEmbeddings
}

func (target selectorlessEmbeddingTarget) transportRoute(path string) transportRoute {
	return transportRoute{
		publicID:       target.publicID,
		localID:        target.localID,
		configFilename: target.configFilename,
		backendMode:    target.backendMode,
		readiness:      embeddingReadinessForModel(path, target.backendMode, firstNonEmpty(target.catalogModel.VLLMTask, target.clusterModel.VLLMTask)),
		section:        routeranalytics.SectionEmbed,
		remote:         target.remote,
		nodeURL:        target.nodeURL,
		rewriteModel:   true,
		insertModel:    true,
		release:        target.release,
		catalogModel:   target.catalogModel,
		clusterModel:   target.clusterModel,
	}
}
