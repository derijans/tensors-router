package proxy

import (
	"fmt"
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
		service.writeClientError(w, http.StatusInternalServerError, "catalog_error", err)
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

type localModelTarget struct {
	model          catalog.Model
	hasModel       bool
	configFilename string
	backendModelID string
	backendMode    string
}

func (service *Service) resolveLocalModelTarget(w http.ResponseWriter, r *http.Request, modelID string, hasModel bool) (localModelTarget, bool) {
	backendMode, err := service.resolveBackendMode("")
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return localModelTarget{}, false
	}
	target := localModelTarget{hasModel: hasModel, backendModelID: modelID, backendMode: backendMode}
	if !hasModel {
		return target, true
	}
	model, ok, err := service.resolveCatalogModelForOpenAIPath(modelID, r.URL.Path)
	if err != nil {
		service.logger.Printf("model catalog check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
		service.writeClientError(w, http.StatusInternalServerError, "catalog_error", err)
		return localModelTarget{}, false
	}
	if !ok {
		service.logger.Printf("unknown model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
		return localModelTarget{}, false
	}
	if !modelSupportsOpenAIPath(model, r.URL.Path) {
		service.logger.Printf("non-llm model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
		return localModelTarget{}, false
	}
	enabled, err := service.localModelEnabled(r.Context(), model.ID)
	if err != nil {
		service.logger.Printf("model state check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
		service.writeClientError(w, http.StatusInternalServerError, "catalog_error", err)
		return localModelTarget{}, false
	}
	if !enabled {
		service.logger.Printf("disabled model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
		return localModelTarget{}, false
	}
	return service.catalogModelTarget(w, modelID, model)
}

func (service *Service) catalogModelTarget(w http.ResponseWriter, modelID string, model catalog.Model) (localModelTarget, bool) {
	backendMode, err := service.catalogModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return localModelTarget{}, false
	}
	target := localModelTarget{model: model, hasModel: true, configFilename: model.Filename, backendModelID: model.ID, backendMode: backendMode}
	if backendMode == BackendModeVLLM {
		target.backendModelID = vllmRequestModelID(modelID, model.ID, model.ServedNames)
	}
	return target, true
}

func (service *Service) forwardLocalModelRequest(w http.ResponseWriter, r *http.Request, body []byte, modelID string, target localModelTarget, insertModel bool) {
	if target.hasModel && target.backendMode == BackendModeLlamaSDCPP && target.model.HasImage && !isEmbeddingsPath(r.URL.Path) {
		if err := service.loadLocalRuntimeForRequest(r.Context(), target.backendMode, target.model.ImageID, target.model.Filename, readinessImage); err != nil {
			service.writeClientError(w, http.StatusBadGateway, "backend_error", err)
			return
		}
	}

	readiness := modelReadiness(r.URL.Path)
	if target.backendMode == BackendModeVLLM {
		readiness = vllmReadinessForTask(r.URL.Path, target.model.VLLMTask)
	}
	requestBody, transformErr := transformBufferedTransportRequestBody(r, body, target.backendModelID, readiness, target.model.ChatTemplate, target.hasModel && target.backendModelID != modelID || insertModel, insertModel)
	if transformErr != nil {
		writeTransportError(w, transformErr)
		return
	}

	requestBody, usageInjected := injectStreamUsageOption(requestBody, r.URL.Path, target.backendMode)

	observed := target.hasModel || isTextInferencePath(r.URL.Path)
	started := time.Now()
	analyticsEvent := service.analytics.newEvent(started, r, requestBody, target.backendModelID, textAnalyticsSection(r.URL.Path), target.backendMode)
	analyticsEvent.PromptBytes = int64(len(body))
	response, workFinalizer, err := service.forwardWithFallbackObserved(r.Context(), r, requestBody, backendForwardTarget{modelID: target.backendModelID, configFilename: target.configFilename, hasModel: target.hasModel, readiness: readiness, mode: target.backendMode})
	if err != nil {
		if observed {
			service.analytics.recordForwardFailure(r.Context(), analyticsEvent, err, workFinalizer)
		}
		service.writeBackendFailure(w, err)
		return
	}
	if target.backendMode == BackendModeVLLM && r.URL.Path == "/v1/responses" {
		response = service.responseWithVLLMTracking(response, vllmResponseTarget{
			publicID:       modelID,
			localID:        target.backendModelID,
			configFilename: target.configFilename,
		})
	}
	if observed {
		response = service.analytics.withResponse(response, analyticsEvent, workFinalizer)
	}
	response = clientStreamUsage(response, usageInjected)

	if err := service.writeModelProxyResponse(w, response, modelID, target.hasModel); err != nil {
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

type requestModelSelection struct {
	modelID     string
	hasModel    bool
	insertModel bool
}

func (service *Service) handleModelRequest(w http.ResponseWriter, r *http.Request, requireModel bool) {
	body, ok := service.readRequestBody(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()

	selection, proceed := service.selectRequestModel(w, r, body)
	if !proceed {
		return
	}
	if requireModel && !selection.hasModel {
		service.logger.Printf("model missing path=%s remote=%s", r.URL.Path, r.RemoteAddr)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if selection.hasModel && service.routeNamedModelRequest(w, r, body, selection.modelID) {
		return
	}
	target, ok := service.resolveLocalModelTarget(w, r, selection.modelID, selection.hasModel)
	if !ok {
		return
	}
	if target.backendMode == BackendModeVLLM && !vllmInferenceAllowed(r.Method, r.URL.Path) {
		openai.WriteEndpointNotFound(w)
		return
	}
	service.forwardLocalModelRequest(w, r, body, selection.modelID, target, selection.insertModel)
}

func (service *Service) selectRequestModel(w http.ResponseWriter, r *http.Request, body []byte) (requestModelSelection, bool) {
	modelID, hasModel, err := modelFromRequest(body, r)
	if err != nil {
		service.logger.Printf("model parse failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return requestModelSelection{}, false
	}
	selection := requestModelSelection{modelID: modelID, hasModel: hasModel}
	switch {
	case hasModel:
		return selection, true
	case isEmbeddingsPath(r.URL.Path):
		return service.selectSelectorlessEmbeddingModel(w, r, body, selection)
	case selectorlessVLLMPath(r.URL.Path):
		modelID, err := service.selectSelectorlessVLLMModel(r.URL.Path)
		if err != nil {
			writeTransportRouteError(w, err)
			return requestModelSelection{}, false
		}
		return requestModelSelection{modelID: modelID, hasModel: true}, true
	default:
		return selection, true
	}
}

func (service *Service) selectSelectorlessEmbeddingModel(w http.ResponseWriter, r *http.Request, body []byte, selection requestModelSelection) (requestModelSelection, bool) {
	target, selected := service.acquireSelectorlessEmbeddingTarget(r.URL.Path, r.Context())
	if !selected {
		return selection, true
	}
	if service.registry != nil {
		acquired := acquiredRegistryRoute{publicID: target.publicID, model: target.clusterModel, route: target.clusterRoute, release: target.release}
		service.handleAcquiredRegistryModelRequest(w, r, body, acquired, true, requestWorkHint{})
		return requestModelSelection{}, false
	}
	return requestModelSelection{modelID: target.publicID, hasModel: true, insertModel: true}, true
}

func (service *Service) routeNamedModelRequest(w http.ResponseWriter, r *http.Request, body []byte, modelID string) bool {
	if service.handleRecipeModelRequest(w, r, body, modelID) {
		return true
	}
	if service.registry != nil && service.registryHasModelForOpenAIPath(modelID, r.URL.Path) {
		service.handleRegistryModelRequest(w, r, body, modelID)
		return true
	}
	return false
}
