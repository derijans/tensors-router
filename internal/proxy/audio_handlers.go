package proxy

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func (service *Service) handleAudioRequest(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		service.logger.Printf("request body read failed path=%s remote=%s error=%v", r.URL.Path, r.RemoteAddr, err)
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body could not be read")
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

	configFilename := ""
	backendModelID := modelID
	selectedBackendMode, err := service.resolveBackendMode("")
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	var selectedModel catalog.Model
	if hasModel {
		model, ok, err := service.catalog.Resolve(modelID)
		if err != nil {
			service.logger.Printf("audio model catalog check failed path=%s model=%q error=%v", r.URL.Path, modelID, err)
			openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
			return
		}
		if !ok {
			service.logger.Printf("audio model not handled by router path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
			hasModel = false
			backendModelID = ""
		} else if !modelSupportsAudioLane(model, lane) {
			service.logger.Printf("unsupported audio model requested path=%s remote=%s model=%q", r.URL.Path, r.RemoteAddr, modelID)
			openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID))
			return
		} else {
			selectedBackendMode, err = service.catalogModelBackendMode(model)
			if err != nil {
				openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
			configFilename = model.Filename
			backendModelID = model.ID
			if selectedBackendMode == BackendModeVLLM {
				backendModelID = vllmRequestModelID(modelID, model.ID, model.ServedNames)
			}
			selectedModel = model
		}
	}
	if selectedBackendMode == BackendModeLlamaSDCPP && isTextToSpeechPath(r.URL.Path) {
		openai.WriteError(w, http.StatusNotImplemented, "unsupported_backend", llamaTextToSpeechUnsupportedMessage)
		return
	}
	if selectedBackendMode == BackendModeLlamaSDCPP {
		if !hasModel || !modelSupportsLlamaAudioPath(selectedModel, r.URL.Path) {
			openai.WriteError(w, http.StatusNotImplemented, "unsupported_backend", "audio route is not supported by the selected split backend config")
			return
		}
	}
	if selectedBackendMode == BackendModeVLLM && !vllmInferenceAllowed(r.Method, r.URL.Path) {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}

	requestBody := body
	if hasModel && requestBodyLooksJSON(body, r) && backendModelID != modelID {
		requestBody = rewriteRequestModel(body, backendModelID)
	}
	analyticsModelID := backendModelID
	if analyticsModelID == "" {
		analyticsModelID = modelID
	}
	started := time.Now()
	analyticsEvent := service.newAnalyticsEvent(started, r, requestBody, analyticsModelID, audioAnalyticsSection(lane), selectedBackendMode)
	readiness := audioReadiness(r.URL.Path, lane, selectedBackendMode)
	if selectedBackendMode == BackendModeLlamaSDCPP && readiness == readinessTranscription {
		requestBody, err = service.adaptBufferedWhisperRequest(r, requestBody)
		if err != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
	}
	response, workFinalizer, err := service.forwardWithFallbackObserved(r.Context(), r, requestBody, backendModelID, configFilename, hasModel, readiness, selectedBackendMode)
	if err != nil {
		status, _, _ := backendFailureResponse(err)
		if analyticsModelID != "" {
			service.recordAnalyticsFailure(analyticsEvent, status, workFinalizer)
		}
		writeBackendFailure(w, err)
		return
	}
	if analyticsModelID != "" {
		response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)
	}
	if err := service.writeProxyResponse(w, response, modelID, false); err != nil {
		return
	}
}
