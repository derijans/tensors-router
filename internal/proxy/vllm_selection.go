package proxy

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

type vllmSelectionCandidate struct {
	publicID string
	loaded   bool
}

func vllmRequestModelID(publicID string, modelID string, servedNames []string) string {
	for _, servedName := range servedNames {
		if strings.TrimSpace(servedName) == publicID {
			return publicID
		}
	}
	return modelID
}

func selectorlessVLLMPath(path string) bool {
	switch path {
	case "/classify", "/score", "/v1/score", "/pooling", "/generative_scoring", "/invocations", "/tokenize", "/detokenize":
		return true
	default:
		return false
	}
}

func (service *Service) selectSelectorlessVLLMModel(path string) (string, error) {
	if service.registry != nil {
		return selectClusterVLLMModel(service.registry.Models(), path)
	}
	if service.catalog == nil {
		return "", transportRouteError{http.StatusServiceUnavailable, "backend_unavailable", "vLLM model catalog is unavailable"}
	}
	models, err := service.catalog.List()
	if err != nil {
		return "", err
	}
	return service.selectCatalogVLLMModel(models, path)
}

func selectClusterVLLMModel(models []cluster.Model, path string) (string, error) {
	candidates := make(map[string]vllmSelectionCandidate)
	for _, model := range models {
		if model.Disabled || !model.Available || model.BackendMode != BackendModeVLLM || !registryModelSupportsOpenAIPath(model, path) {
			continue
		}
		identity := firstNonEmpty(model.ConfigHash, model.Filename+"\x00"+model.LocalID)
		candidate := candidates[identity]
		if candidate.publicID == "" || model.PublicID == model.LocalID {
			candidate.publicID = model.PublicID
		}
		candidate.loaded = candidate.loaded || model.Loaded
		candidates[identity] = candidate
	}
	return chooseVLLMSelection(candidates, path)
}

func (service *Service) selectCatalogVLLMModel(models []catalog.Model, path string) (string, error) {
	loadedFilenames := map[backendReadiness]string{}
	for _, readiness := range []backendReadiness{readinessText, readinessEmbeddings} {
		if runtime, err := service.runtimeForBackendMode(BackendModeVLLM, readiness); err == nil && runtime != nil {
			runtime.state.mu.Lock()
			if !runtime.state.switching {
				loadedFilenames[readiness] = runtime.state.filename
			}
			runtime.state.mu.Unlock()
		}
	}
	candidates := make(map[string]vllmSelectionCandidate)
	for _, model := range models {
		mode, err := service.catalogModelBackendMode(model)
		if err != nil || mode != BackendModeVLLM || !modelSupportsOpenAIPath(model, path) {
			continue
		}
		readiness := vllmReadinessForTask(path, model.VLLMTask)
		loadedFilename := loadedFilenames[readiness]
		candidates[model.Filename] = vllmSelectionCandidate{publicID: model.ID, loaded: loadedFilename != "" && model.Filename == loadedFilename}
	}
	return chooseVLLMSelection(candidates, path)
}

func chooseVLLMSelection(candidates map[string]vllmSelectionCandidate, path string) (string, error) {
	loaded := make([]string, 0)
	available := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		available = append(available, candidate.publicID)
		if candidate.loaded {
			loaded = append(loaded, candidate.publicID)
		}
	}
	sort.Strings(loaded)
	sort.Strings(available)
	if len(loaded) == 1 {
		return loaded[0], nil
	}
	if len(loaded) > 1 || len(available) > 1 {
		return "", transportRouteError{http.StatusBadRequest, "ambiguous_model_selector", fmt.Sprintf("%s requires a model selector because multiple compatible vLLM models are available: %s", path, strings.Join(available, ", "))}
	}
	if len(available) == 1 {
		return available[0], nil
	}
	return "", transportRouteError{http.StatusServiceUnavailable, "backend_unavailable", fmt.Sprintf("%s has no compatible vLLM model", path)}
}
