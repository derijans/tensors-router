package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/backendmode"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/recipes"
)

func (service *Service) handleRegistryModelRequest(w http.ResponseWriter, r *http.Request, body []byte, publicID string) {
	model, route, release, ok := service.acquireRegistryModelRoute(r, publicID)
	defer release()
	modelBackendMode, backendModeErr := service.clusterModelBackendMode(model)
	if backendModeErr != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", backendModeErr.Error())
		return
	}
	if modelBackendMode == BackendModeVLLM && !vllmInferenceAllowed(r.Method, r.URL.Path) {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if !registryModelSupportsOpenAIPath(model, r.URL.Path) {
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", publicID))
		return
	}
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("model %q has no available replicas", publicID))
		return
	}
	backendModelID := route.LocalID
	if modelBackendMode == BackendModeVLLM {
		backendModelID = vllmRequestModelID(publicID, route.LocalID, model.ServedNames)
	}

	profile := service.localChatTemplateProfile(route.Filename, route.Remote)
	readiness := modelReadiness(r.URL.Path)
	if modelBackendMode == BackendModeVLLM {
		readiness = vllmReadinessForTask(r.URL.Path, model.VLLMTask)
	}
	requestBody, transformErr := transformBufferedTransportRequestBody(r, body, backendModelID, readiness, profile, true)
	if transformErr != nil {
		writeTransportError(w, transformErr)
		return
	}
	var response *http.Response
	var err error
	var analyticsEvent routeranalytics.Event
	var workFinalizer analyticsEventFinalizer
	recordAnalytics := false
	if route.Remote {
		response, err = service.forwardRemote(r.Context(), r, requestBody, route)
	} else {
		routeBackendMode, err := service.clusterRouteBackendMode(route, model)
		if err != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if routeBackendMode == BackendModeLlamaSDCPP && model.HasImage && !isEmbeddingsPath(r.URL.Path) {
			if err := service.loadLocalRuntimeForRequest(r.Context(), routeBackendMode, route.PublicImageID, route.Filename, readinessImage); err != nil {
				writeBackendFailure(w, err)
				return
			}
		}
		started := time.Now()
		analyticsEvent = service.newAnalyticsEvent(started, r, requestBody, backendModelID, textAnalyticsSection(r.URL.Path), routeBackendMode)
		recordAnalytics = true
		forwardModelID := route.PublicID
		if routeBackendMode == BackendModeVLLM {
			forwardModelID = backendModelID
		}
		response, workFinalizer, err = service.forwardWithFallbackObserved(r.Context(), r, requestBody, forwardModelID, route.Filename, true, readiness, routeBackendMode)
	}
	if err != nil {
		status, _, _ := backendFailureResponse(err)
		if recordAnalytics {
			service.recordAnalyticsFailure(analyticsEvent, status, workFinalizer)
		}
		writeBackendFailure(w, err)
		return
	}
	if modelBackendMode == BackendModeVLLM && r.URL.Path == "/v1/responses" {
		response = service.responseWithVLLMTracking(response, vllmResponseTarget{
			publicID:       publicID,
			localID:        backendModelID,
			configFilename: route.Filename,
			remote:         route.Remote,
			nodeURL:        route.NodeURL,
		})
	}
	response = responseWithRelease(response, release)
	if recordAnalytics {
		response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)
	}

	if err := service.writeModelProxyResponse(w, response, publicID, true); err != nil {
		return
	}
}

func (service *Service) acquireRegistryModelRoute(r *http.Request, publicID string) (cluster.Model, cluster.Route, func(), bool) {
	if isEmbeddingsPath(r.URL.Path) {
		model, ok := service.registry.EmbeddingModel(publicID)
		if !ok {
			return cluster.Model{}, cluster.Route{}, func() {}, false
		}
		modelBackendMode, err := service.clusterModelBackendMode(model)
		if err != nil {
			return cluster.Model{}, cluster.Route{}, func() {}, false
		}
		route, release, ok := service.registry.AcquireEmbedding(publicID, service.localBackendAvailableForRoute(r.Context(), modelBackendMode, readinessEmbeddings))
		return model, route, release, ok
	}
	model, ok := service.registry.Model(publicID)
	if !ok {
		return cluster.Model{}, cluster.Route{}, func() {}, false
	}
	modelBackendMode, err := service.clusterModelBackendMode(model)
	if err != nil {
		return cluster.Model{}, cluster.Route{}, func() {}, false
	}
	readiness := readinessText
	if modelBackendMode == BackendModeVLLM {
		readiness = vllmReadinessForTask(r.URL.Path, model.VLLMTask)
	}
	route, release, ok := service.registry.Acquire(publicID, service.localBackendAvailableForRoute(r.Context(), modelBackendMode, readiness))
	return model, route, release, ok
}

func (service *Service) registryHasModelForOpenAIPath(modelID string, path string) bool {
	if isEmbeddingsPath(path) {
		return service.registry.HasEmbeddingModel(modelID)
	}
	return service.registry.HasModel(modelID)
}

func (service *Service) handleRegistryImageRequest(w http.ResponseWriter, r *http.Request, body []byte, publicImageID string) bool {
	activeConfigFilename := service.imageCatalogConfigSelector()
	model, hasImageModel := service.registry.ImageModel(publicImageID, activeConfigFilename)
	if !hasImageModel {
		return false
	}

	modelBackendMode, err := service.clusterModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return true
	}
	jobBackendMode := modelBackendMode
	route, release, ok := service.registry.AcquireImage(publicImageID, service.localBackendAvailableForRoute(r.Context(), modelBackendMode, readinessImage), activeConfigFilename)
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("image model %q has no available replicas", publicImageID))
		return true
	}

	request := rewriteImageRequest(r, publicImageID, route.LocalImageID)
	requestBody := rewriteImageRequestBody(body, publicImageID, route.LocalImageID, r)
	var response *http.Response
	var forwardErr error
	var analyticsEvent routeranalytics.Event
	var workFinalizer analyticsEventFinalizer
	recordAnalytics := false
	if route.Remote {
		response, forwardErr = service.forwardRemote(r.Context(), request, requestBody, route)
	} else {
		routeBackendMode, err := service.clusterRouteBackendMode(route, model)
		if err != nil {
			release()
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return true
		}
		jobBackendMode = routeBackendMode
		if routeBackendMode == BackendModeLlamaSDCPP && clusterModelNeedsPrimaryTextRuntime(model) {
			if err := service.loadLocalRuntimeForRequest(r.Context(), routeBackendMode, route.PublicID, route.Filename, readinessText); err != nil {
				release()
				openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
				return true
			}
		}
		started := time.Now()
		analyticsEvent = service.newAnalyticsEvent(started, request, requestBody, route.LocalImageID, routeranalytics.SectionImage, routeBackendMode)
		recordAnalytics = true
		response, workFinalizer, forwardErr = service.forwardWithFallbackObserved(r.Context(), request, requestBody, route.PublicImageID, route.Filename, true, readinessImage, routeBackendMode)
	}
	if forwardErr != nil {
		release()
		if recordAnalytics {
			service.recordAnalyticsFailure(analyticsEvent, http.StatusBadGateway, workFinalizer)
		}
		openai.WriteError(w, http.StatusBadGateway, "backend_error", forwardErr.Error())
		return true
	}
	if isSdcppJobSubmissionPath(r.URL.Path) {
		response = service.responseWithSdcppJobTracking(response, sdcppJobTarget{
			publicImageID:  publicImageID,
			configFilename: route.Filename,
			backendMode:    jobBackendMode,
			remote:         route.Remote,
			nodeURL:        route.NodeURL,
		})
	}
	response = responseWithRelease(response, release)
	if recordAnalytics {
		response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)
	}

	if err := service.writeProxyResponse(w, response, publicImageID, true); err != nil {
		return true
	}
	return true
}

func (service *Service) handleRegistryImageOptions(w http.ResponseWriter, r *http.Request, body []byte, publicImageID string) bool {
	activeConfigFilename := service.imageCatalogConfigSelector()
	model, hasImageModel := service.registry.ImageModel(publicImageID, activeConfigFilename)
	if !hasImageModel {
		return false
	}

	modelBackendMode, err := service.clusterModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return true
	}
	route, release, ok := service.registry.AcquireImage(publicImageID, service.localBackendAvailableForRoute(r.Context(), modelBackendMode, readinessImage), activeConfigFilename)
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("image model %q has no available replicas", publicImageID))
		return true
	}

	request := rewriteImageRequest(r, publicImageID, route.LocalImageID)
	requestBody := rewriteImageRequestBody(body, publicImageID, route.LocalImageID, r)
	if route.Remote {
		response, err := service.forwardRemote(r.Context(), request, requestBody, route)
		if err != nil {
			release()
			openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
			return true
		}
		response = responseWithRelease(response, release)
		if err := service.writeProxyResponse(w, response, publicImageID, true); err != nil {
			return true
		}
		return true
	}

	modelContext, cancelModelContext := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancelModelContext()
	routeBackendMode, err := service.clusterRouteBackendMode(route, model)
	if err != nil {
		release()
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return true
	}
	if routeBackendMode == BackendModeLlamaSDCPP && clusterModelNeedsPrimaryTextRuntime(model) {
		if err := service.loadLocalConfig(modelContext, routeBackendMode, route.PublicID, route.Filename, readinessText); err != nil {
			release()
			openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
			return true
		}
	}
	_, releaseModel, _, err := service.acquireModelConfigForBackendMode(routeBackendMode, modelContext, route.PublicImageID, route.Filename, readinessImage, false)
	if err != nil {
		release()
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return true
	}
	releaseModel()
	release()
	openai.WriteJSON(w, http.StatusOK, map[string]any{})
	return true
}

func (service *Service) handleRegistryAudioRequest(w http.ResponseWriter, r *http.Request, body []byte, publicID string, lane string, selected *cluster.Model) {
	model, ok := service.registryAudioModel(publicID, lane)
	if selected != nil {
		model = *selected
		ok = true
	}
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", publicID))
		return
	}
	modelBackendMode, err := service.clusterModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if modelBackendMode == BackendModeVLLM && !vllmInferenceAllowed(r.Method, r.URL.Path) {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	readiness := audioReadiness(r.URL.Path, lane, modelBackendMode)
	var route cluster.Route
	var release func()
	if selected != nil {
		route, release, ok = service.registry.AcquireSpecificVoice(publicID, selected.NodeID, selected.Filename, service.localBackendAvailableForRoute(r.Context(), modelBackendMode, readiness))
	} else {
		route, release, ok = service.acquireRegistryAudioRoute(r, publicID, lane, modelBackendMode, readiness)
	}
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("model %q has no available replicas", publicID))
		return
	}

	backendModelID := route.LocalID
	if modelBackendMode == BackendModeVLLM {
		backendModelID = vllmRequestModelID(publicID, route.LocalID, model.ServedNames)
	}
	requestBody := body
	if requestBodyLooksJSON(body, r) {
		requestBody = rewriteRequestModel(body, backendModelID)
	}
	var response *http.Response
	var analyticsEvent routeranalytics.Event
	var workFinalizer analyticsEventFinalizer
	recordAnalytics := false
	if route.Remote {
		response, err = service.forwardRemote(r.Context(), r, requestBody, route)
	} else {
		routeBackendMode, modeErr := service.resolveBackendMode(route.BackendMode)
		if modeErr != nil {
			release()
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", modeErr.Error())
			return
		}
		if routeBackendMode == BackendModeLlamaSDCPP {
			if lane == recipes.KindMusic || !clusterModelSupportsLlamaAudioPath(model, r.URL.Path) {
				release()
				openai.WriteError(w, http.StatusNotImplemented, "unsupported_backend", "audio route is not supported by the selected split backend config")
				return
			}
		}
		started := time.Now()
		analyticsEvent = service.newAnalyticsEvent(started, r, requestBody, backendModelID, audioAnalyticsSection(lane), routeBackendMode)
		recordAnalytics = true
		readiness = audioReadiness(r.URL.Path, lane, routeBackendMode)
		forwardModelID := route.PublicID
		if routeBackendMode == BackendModeVLLM {
			forwardModelID = backendModelID
		}
		response, workFinalizer, err = service.forwardWithFallbackObserved(r.Context(), r, requestBody, forwardModelID, route.Filename, true, readiness, routeBackendMode)
	}
	if err != nil {
		release()
		if recordAnalytics {
			service.recordAnalyticsFailure(analyticsEvent, http.StatusBadGateway, workFinalizer)
		}
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	response = responseWithRelease(response, release)
	if recordAnalytics {
		response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)
	}
	if err := service.writeProxyResponse(w, response, publicID, false); err != nil {
		return
	}
}

func (service *Service) acquireRegistryAudioRoute(r *http.Request, publicID string, lane string, backendMode string, readiness backendReadiness) (cluster.Route, func(), bool) {
	if lane == recipes.KindMusic {
		return service.registry.AcquireMusic(publicID, service.localBackendAvailableForRoute(r.Context(), BackendModeKobold, readiness))
	}
	return service.registry.AcquireVoice(publicID, service.localBackendAvailableForRoute(r.Context(), backendMode, readiness))
}

func (service *Service) registryHasAudioModel(publicID string, lane string) bool {
	if lane == recipes.KindMusic {
		return service.registry.HasMusicModel(publicID)
	}
	return service.registry.HasVoiceModel(publicID)
}

func (service *Service) registryAudioModel(publicID string, lane string) (cluster.Model, bool) {
	if lane == recipes.KindMusic {
		return service.registry.MusicModel(publicID)
	}
	return service.registry.VoiceModel(publicID)
}

func clusterModelSupportsLlamaAudioPath(model cluster.Model, path string) bool {
	if model.Capabilities.Voice == nil {
		return false
	}
	switch path {
	case "/v1/audio/speech":
		return strings.TrimSpace(model.Capabilities.Voice.TalkerModel) != ""
	case "/v1/audio/transcriptions":
		return strings.TrimSpace(model.Capabilities.Voice.WhisperModel) != ""
	default:
		return false
	}
}

func (service *Service) forwardRemote(ctx context.Context, original *http.Request, body []byte, route cluster.Route) (*http.Response, error) {
	baseURL, err := service.clusterClient.AuthorizedBaseURL(route.NodeURL)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	target.Path = joinPath(target.Path, "/router/v1/node/inference"+original.URL.Path)
	target.RawQuery = original.URL.RawQuery

	request, err := http.NewRequestWithContext(ctx, original.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyClusterRequestHeaders(request.Header, original.Header)
	request.Header.Set("Authorization", "Bearer "+service.clusterToken)
	request.Host = target.Host
	return service.client.Do(request)
}

func rewriteRequestModel(body []byte, modelID string) []byte {
	if strings.TrimSpace(modelID) == "" || len(body) == 0 {
		return body
	}
	rewritten := rewriteJSONModel(body, modelID)
	return rewritten
}

func rewriteImageRequest(original *http.Request, publicImageID string, localImageID string) *http.Request {
	if strings.TrimSpace(publicImageID) == "" || strings.TrimSpace(localImageID) == "" || publicImageID == localImageID {
		return original
	}

	rewritten := original.Clone(original.Context())
	targetURL := *original.URL
	values := targetURL.Query()
	queryChanged := rewriteQuerySelector(values, "model", publicImageID, localImageID)
	queryChanged = rewriteQuerySelector(values, "sd_model_checkpoint", publicImageID, localImageID) || queryChanged
	if queryChanged {
		targetURL.RawQuery = values.Encode()
	}
	rewritten.URL = &targetURL
	rewritten.Header = original.Header.Clone()
	if strings.TrimSpace(rewritten.Header.Get("X-Tensors-Model")) == publicImageID {
		rewritten.Header.Set("X-Tensors-Model", localImageID)
	}
	return rewritten
}

func rewriteImageRequestBody(body []byte, publicImageID string, localImageID string, r *http.Request) []byte {
	if strings.TrimSpace(publicImageID) == "" || strings.TrimSpace(localImageID) == "" || publicImageID == localImageID {
		return body
	}
	if len(body) == 0 || !requestBodyLooksJSON(body, r) {
		return body
	}

	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		return body
	}
	changed := rewriteMapSelector(value, "model", publicImageID, localImageID)
	changed = rewriteMapSelector(value, "sd_model_checkpoint", publicImageID, localImageID) || changed
	if overrideSettings, ok := value["override_settings"].(map[string]any); ok {
		changed = rewriteMapSelector(overrideSettings, "sd_model_checkpoint", publicImageID, localImageID) || changed
	}
	if !changed {
		return body
	}
	rewritten, err := json.Marshal(value)
	if err != nil {
		return body
	}
	return rewritten
}

func rewriteQuerySelector(values url.Values, key string, publicImageID string, localImageID string) bool {
	changed := false
	for index, value := range values[key] {
		if strings.TrimSpace(value) == publicImageID {
			values[key][index] = localImageID
			changed = true
		}
	}
	return changed
}

func rewriteMapSelector(value map[string]any, key string, publicImageID string, localImageID string) bool {
	text, ok := value[key].(string)
	if !ok || strings.TrimSpace(text) != publicImageID {
		return false
	}
	value[key] = localImageID
	return true
}

func clusterImageModelObjects(models []cluster.Model, activeConfigFilename string) []imageModelObject {
	seen := map[string]struct{}{}
	response := make([]imageModelObject, 0, len(models))
	for _, model := range models {
		if !clusterImageModelVisible(model, activeConfigFilename) {
			continue
		}
		if _, ok := seen[model.PublicImageID]; ok {
			continue
		}
		seen[model.PublicImageID] = struct{}{}
		filename := ""
		if model.Capabilities.Image != nil {
			filename = model.Capabilities.Image.Model
		}
		response = append(response, imageModelObject{
			Title:     model.PublicImageID,
			ModelName: model.PublicImageID,
			Hash:      "",
			SHA256:    "",
			Filename:  filename,
			Config:    model.Filename,
		})
	}
	return response
}

func clusterImageModelVisible(model cluster.Model, activeConfigFilename string) bool {
	if model.Disabled || !model.HasImage || model.PublicImageID == "" {
		return false
	}
	if !model.HasLLM {
		return true
	}
	if model.BackendMode == BackendModeLlamaSDCPP {
		return true
	}
	if model.Source != cluster.SourceMaster && model.Source != cluster.SourceLocal {
		return false
	}
	return model.Filename == activeConfigFilename
}

func modelSupportsOpenAIPath(model catalog.Model, path string) bool {
	if backendmode.Normalize(model.BackendMode) == BackendModeVLLM {
		return vllmTaskSupportsPath(model.VLLMTask, path)
	}
	if isEmbeddingsPath(path) {
		if path == "/api/embed" {
			return model.HasEmbeddings
		}
		return model.HasEmbeddings || model.HasLLM
	}
	if isCorePath(path) {
		return model.HasLLM
	}
	return modelSupportsTextLane(model)
}

func modelSupportsAudioLane(model catalog.Model, lane string) bool {
	if lane == recipes.KindMusic {
		return model.HasMusic
	}
	return model.HasVoice
}

func modelSupportsTextLane(model catalog.Model) bool {
	return model.HasLLM || model.HasEmbeddings || model.HasMultimodal
}

func modelNeedsPrimaryTextRuntime(model catalog.Model) bool {
	separateEmbeddings := model.Capabilities.Embeddings != nil && model.Capabilities.Embeddings.Separate
	return model.HasLLM || model.HasMultimodal || (model.HasEmbeddings && !separateEmbeddings)
}

func clusterModelNeedsPrimaryTextRuntime(model cluster.Model) bool {
	separateEmbeddings := model.Capabilities.Embeddings != nil && model.Capabilities.Embeddings.Separate
	return model.HasLLM || model.HasMultimodal || (model.HasEmbeddings && !separateEmbeddings)
}

func registryModelSupportsOpenAIPath(model cluster.Model, path string) bool {
	if backendmode.Normalize(model.BackendMode) == BackendModeVLLM {
		return vllmTaskSupportsPath(model.VLLMTask, path)
	}
	if isEmbeddingsPath(path) {
		if path == "/api/embed" {
			return model.HasEmbeddings
		}
		return model.HasEmbeddings || model.HasLLM
	}
	if isCorePath(path) {
		return model.HasLLM
	}
	return model.HasLLM || model.HasEmbeddings || model.HasMultimodal
}
