package proxy

import (
	"fmt"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func (service *Service) handleAudioRequest(w http.ResponseWriter, r *http.Request) {
	body, ok := service.readRequestBody(w, r)
	if !ok {
		return
	}
	defer r.Body.Close()

	lane := audioRouteKind(r.URL.Path)
	modelID, hasModel, err := audioModelFromRequest(body, r)
	if err != nil {
		service.logger.Printf("audio model parse failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	var automaticallySelected *cluster.Model
	if !hasModel && isSTTPath(r.URL.Path) {
		selected, selectedOK := service.selectAutomaticSTTModel(r.Context())
		if selectedOK {
			modelID = firstNonEmpty(selected.PublicID, selected.LocalID)
			hasModel = true
			automaticallySelected = &selected
		}
		if !hasModel {
			openai.WriteError(w, http.StatusServiceUnavailable, "backend_error", "no compatible transcription configuration is available")
			return
		}
	}

	if hasModel && service.handleRecipeAudioRequest(w, r, body, modelID, lane) {
		return
	}
	if hasModel && service.registry != nil && service.registryHasAudioModel(modelID, lane) {
		service.handleRegistryAudioRequest(w, r, body, modelID, lane, automaticallySelected)
		return
	}

	target, ok := service.resolveLocalAudioTarget(w, r, modelID, hasModel, lane)
	if !ok || service.rejectUnsupportedAudioBackend(w, r, target) {
		return
	}
	service.forwardLocalAudioRequest(w, r, body, modelID, lane, target)
}

func (service *Service) resolveLocalAudioTarget(w http.ResponseWriter, r *http.Request, modelID string, hasModel bool, lane string) (localModelTarget, bool) {
	backendMode, err := service.resolveBackendMode("")
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return localModelTarget{}, false
	}
	target := localModelTarget{hasModel: hasModel, backendModelID: modelID, backendMode: backendMode}
	if !hasModel {
		return target, true
	}
	model, found, err := service.catalog.Resolve(modelID)
	if err != nil {
		service.logger.Printf("audio model catalog check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
		service.writeClientError(w, http.StatusInternalServerError, "catalog_error", err)
		return localModelTarget{}, false
	}
	if !found {
		service.logger.Printf("audio model not handled by router path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
		target.hasModel = false
		target.backendModelID = ""
		return target, true
	}
	if !modelSupportsAudioLane(model, lane) {
		service.logger.Printf("unsupported audio model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
		return localModelTarget{}, false
	}
	return service.catalogModelTarget(w, modelID, model)
}

func (service *Service) rejectUnsupportedAudioBackend(w http.ResponseWriter, r *http.Request, target localModelTarget) bool {
	switch {
	case target.backendMode == BackendModeLlamaSDCPP && isTextToSpeechPath(r.URL.Path):
		openai.WriteError(w, http.StatusNotImplemented, "unsupported_backend", llamaTextToSpeechUnsupportedMessage)
	case target.backendMode == BackendModeLlamaSDCPP && (!target.hasModel || !modelSupportsLlamaAudioPath(target.model, r.URL.Path)):
		openai.WriteError(w, http.StatusNotImplemented, "unsupported_backend", "audio route is not supported by the selected split backend config")
	case target.backendMode == BackendModeVLLM && !vllmInferenceAllowed(r.Method, r.URL.Path):
		openai.WriteEndpointNotFound(w)
	default:
		return false
	}
	return true
}

func (service *Service) forwardLocalAudioRequest(w http.ResponseWriter, r *http.Request, body []byte, modelID string, lane string, target localModelTarget) {
	requestBody := body
	if target.hasModel && requestBodyLooksJSON(body, r) && target.backendModelID != modelID {
		requestBody = rewriteRequestModel(body, target.backendModelID)
	}
	analyticsModelID := target.backendModelID
	if analyticsModelID == "" {
		analyticsModelID = modelID
	}
	started := time.Now()
	analyticsEvent := service.analytics.newEvent(started, r, requestBody, analyticsModelID, audioAnalyticsSection(lane), target.backendMode)
	readiness := audioReadiness(r.URL.Path, lane, target.backendMode)
	if target.backendMode == BackendModeLlamaSDCPP && readiness == readinessTranscription {
		adapted, err := service.adaptBufferedWhisperRequest(r, requestBody)
		if err != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		requestBody = adapted
	}
	response, workFinalizer, err := service.forwardWithFallbackObserved(r.Context(), r, requestBody, backendForwardTarget{modelID: target.backendModelID, configFilename: target.configFilename, hasModel: target.hasModel, readiness: readiness, mode: target.backendMode})
	if err != nil {
		if analyticsModelID != "" {
			service.analytics.recordForwardFailure(r.Context(), analyticsEvent, err, workFinalizer)
		}
		service.writeBackendFailure(w, err)
		return
	}
	if analyticsModelID != "" {
		response = service.analytics.withResponse(response, analyticsEvent, workFinalizer)
	}
	if err := service.writeProxyResponse(w, response, modelID, false); err != nil {
		return
	}
}
