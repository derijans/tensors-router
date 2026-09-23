package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"tensors-router/internal/backendmode"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func (service *Service) handleModels(w http.ResponseWriter) {
	if service.registry != nil {
		openai.WriteJSON(w, http.StatusOK, openai.ModelsResponseFromCatalog(cluster.PublicCatalogModels(service.registry.Models())))
		return
	}

	models, err := service.catalog.List()
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	visible := make([]catalog.Model, 0, len(models))
	for _, model := range models {
		mode, modeErr := service.catalogModelBackendMode(model)
		if model.HasLLM || modeErr == nil && mode == BackendModeVLLM && (model.HasEmbeddings || model.HasVoice) {
			visible = append(visible, model)
		}
	}
	openai.WriteJSON(w, http.StatusOK, openai.ModelsResponseFromCatalog(visible))
}

func (service *Service) handleModelRequest(w http.ResponseWriter, r *http.Request, requireModel bool) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		service.logger.Printf("request body read failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body could not be read")
		return
	}
	defer r.Body.Close()

	modelID, hasModel, err := modelFromRequest(body, r)
	if err != nil {
		service.logger.Printf("model parse failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	insertModel := false
	if !hasModel && isEmbeddingsPath(r.URL.Path) {
		if target, selected := service.acquireSelectorlessEmbeddingTarget(r.URL.Path, r.Context()); selected {
			if service.registry != nil {
				service.handleAcquiredRegistryModelRequest(w, r, body, target.publicID, target.clusterModel, target.clusterRoute, target.release, true, requestWorkHint{})
				return
			}
			modelID = target.publicID
			hasModel = true
			insertModel = true
		}
	}
	if !hasModel && !isEmbeddingsPath(r.URL.Path) && selectorlessVLLMPath(r.URL.Path) {
		modelID, err = service.selectSelectorlessVLLMModel(r.URL.Path)
		if err != nil {
			writeTransportRouteError(w, err)
			return
		}
		hasModel = true
	}
	if requireModel && !hasModel {
		service.logger.Printf("model missing path=%s remote=%s", r.URL.Path, r.RemoteAddr)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	if hasModel && service.handleRecipeModelRequest(w, r, body, modelID) {
		return
	}

	if hasModel && service.registry != nil && service.registryHasModelForOpenAIPath(modelID, r.URL.Path) {
		service.handleRegistryModelRequest(w, r, body, modelID)
		return
	}

	configFilename := ""
	backendModelID := modelID
	selectedBackendMode, err := service.resolveBackendMode("")
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	var selectedModel catalog.Model
	if hasModel {
		model, ok, err := service.resolveCatalogModelForOpenAIPath(modelID, r.URL.Path)
		if err != nil {
			service.logger.Printf("model catalog check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
			openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
			return
		}
		if !ok {
			service.logger.Printf("unknown model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
			openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
			return
		}
		if !modelSupportsOpenAIPath(model, r.URL.Path) {
			service.logger.Printf("non-llm model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
			openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
			return
		}
		enabled, err := service.localModelEnabled(r.Context(), model.ID)
		if err != nil {
			service.logger.Printf("model state check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
			openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
			return
		}
		if !enabled {
			service.logger.Printf("disabled model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
			openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
			return
		}
		configFilename = model.Filename
		backendModelID = model.ID
		selectedModel = model
		selectedBackendMode, err = service.catalogModelBackendMode(model)
		if err != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if selectedBackendMode == BackendModeVLLM {
			backendModelID = vllmRequestModelID(modelID, model.ID, model.ServedNames)
		}
	}
	if selectedBackendMode == BackendModeVLLM && !vllmInferenceAllowed(r.Method, r.URL.Path) {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}

	if hasModel && selectedBackendMode == BackendModeLlamaSDCPP && selectedModel.HasImage && !isEmbeddingsPath(r.URL.Path) {
		if err := service.loadLocalRuntimeForRequest(r.Context(), selectedBackendMode, selectedModel.ImageID, selectedModel.Filename, readinessImage); err != nil {
			openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
			return
		}
	}

	readiness := modelReadiness(r.URL.Path)
	if selectedBackendMode == BackendModeVLLM {
		readiness = vllmReadinessForTask(r.URL.Path, selectedModel.VLLMTask)
	}
	requestBody, transformErr := transformBufferedTransportRequestBody(r, body, backendModelID, readiness, selectedModel.ChatTemplate, hasModel && backendModelID != modelID || insertModel, insertModel)
	if transformErr != nil {
		writeTransportError(w, transformErr)
		return
	}

	requestBody, usageInjected := injectStreamUsageOption(requestBody, r.URL.Path, selectedBackendMode)

	started := time.Now()
	analyticsEvent := service.newAnalyticsEvent(started, r, requestBody, backendModelID, textAnalyticsSection(r.URL.Path), selectedBackendMode)
	analyticsEvent.PromptBytes = int64(len(body))
	response, workFinalizer, err := service.forwardWithFallbackObserved(r.Context(), r, requestBody, backendModelID, configFilename, hasModel, readiness, selectedBackendMode)
	if err != nil {
		status, _, _ := backendFailureResponse(err)
		if hasModel || isTextInferencePath(r.URL.Path) {
			service.recordAnalyticsFailure(analyticsEvent, status, workFinalizer)
		}
		writeBackendFailure(w, err)
		return
	}
	if selectedBackendMode == BackendModeVLLM && r.URL.Path == "/v1/responses" {
		response = service.responseWithVLLMTracking(response, vllmResponseTarget{
			publicID:       modelID,
			localID:        backendModelID,
			configFilename: configFilename,
		})
	}
	if hasModel || isTextInferencePath(r.URL.Path) {
		response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)
	}
	response = responseWithoutInjectedUsage(response, usageInjected)

	if err := service.writeModelProxyResponse(w, response, modelID, hasModel); err != nil {
		return
	}
}

func (service *Service) resolveCatalogModel(modelID string) (catalog.Model, bool, error) {
	return service.catalog.Resolve(modelID)
}

func (service *Service) resolveCatalogModelForOpenAIPath(modelID string, path string) (catalog.Model, bool, error) {
	model, ok, err := service.catalog.Resolve(modelID)
	if err != nil || ok || !isEmbeddingsPath(path) {
		return model, ok, err
	}
	model, ok, err = service.resolveVisibleImageModel(modelID)
	if err != nil || !ok {
		return catalog.Model{}, ok, err
	}
	if model.HasEmbeddings {
		return model, true, nil
	}
	return catalog.Model{}, false, nil
}

func (service *Service) catalogModelBackendMode(model catalog.Model) (string, error) {
	return backendmode.Resolve(model.BackendMode, service.backendMode)
}

func (service *Service) clusterModelBackendMode(model cluster.Model) (string, error) {
	return service.resolveBackendMode(model.BackendMode)
}

func (service *Service) clusterRouteBackendMode(route cluster.Route, model cluster.Model) (string, error) {
	if strings.TrimSpace(route.BackendMode) != "" {
		return service.resolveBackendMode(route.BackendMode)
	}
	return service.clusterModelBackendMode(model)
}
