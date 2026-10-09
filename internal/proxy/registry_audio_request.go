package proxy

import (
	"fmt"
	"net/http"
	"time"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/recipes"
)

type registryAudioRequest struct {
	acquired         acquiredRegistryRoute
	lane             string
	modelBackendMode string
	backendModelID   string
	requestBody      []byte
}

func (service *Service) handleRegistryAudioRequest(w http.ResponseWriter, r *http.Request, body []byte, publicID string, lane string, selected *cluster.Model) {
	audio, ok := service.acquireRegistryAudioRequest(w, r, publicID, lane, selected)
	if !ok {
		return
	}
	audio.backendModelID = audio.acquired.route.LocalID
	if audio.modelBackendMode == BackendModeVLLM {
		audio.backendModelID = vllmRequestModelID(publicID, audio.acquired.route.LocalID, audio.acquired.model.ServedNames)
	}
	audio.requestBody = body
	if requestBodyLooksJSON(body, r) {
		audio.requestBody = rewriteRequestModel(body, audio.backendModelID)
	}
	forwarded, ok := service.forwardRegistryAudioRequest(w, r, audio)
	if !ok {
		return
	}
	response := responseWithRelease(forwarded.response, audio.acquired.release)
	response = forwarded.tracking.wrap(response)
	_ = service.writeProxyResponse(w, response, publicID, false)
}

func (service *Service) acquireRegistryAudioRequest(w http.ResponseWriter, r *http.Request, publicID string, lane string, selected *cluster.Model) (registryAudioRequest, bool) {
	model, ok := service.registryAudioModel(publicID, lane)
	if selected != nil {
		model, ok = *selected, true
	}
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", publicID))
		return registryAudioRequest{}, false
	}
	modelBackendMode, err := service.clusterModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return registryAudioRequest{}, false
	}
	if modelBackendMode == BackendModeVLLM && !vllmInferenceAllowed(r.Method, r.URL.Path) {
		openai.WriteEndpointNotFound(w)
		return registryAudioRequest{}, false
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
		return registryAudioRequest{}, false
	}
	return registryAudioRequest{
		acquired:         acquiredRegistryRoute{publicID: publicID, model: model, route: route, release: release},
		lane:             lane,
		modelBackendMode: modelBackendMode,
	}, true
}

func (service *Service) forwardRegistryAudioRequest(w http.ResponseWriter, r *http.Request, audio registryAudioRequest) (registryForward, bool) {
	var forwarded registryForward
	var err error
	if audio.acquired.route.Remote {
		forwarded.response, err = service.forwardRemote(r.Context(), r, audio.requestBody, audio.acquired.route)
	} else {
		routeBackendMode, ok := service.admitLocalRegistryAudioRequest(w, r, audio)
		if !ok {
			return forwarded, false
		}
		forwarded.response, forwarded.tracking, err = service.forwardLocalRegistryAudioRequest(r, audio, routeBackendMode)
	}
	if err != nil {
		audio.acquired.release()
		forwarded.tracking.recordFailure(r.Context(), err)
		service.writeClientError(w, http.StatusBadGateway, "backend_error", err)
		return forwarded, false
	}
	return forwarded, true
}

func (service *Service) admitLocalRegistryAudioRequest(w http.ResponseWriter, r *http.Request, audio registryAudioRequest) (string, bool) {
	routeBackendMode, err := service.resolveBackendMode(audio.acquired.route.BackendMode)
	if err != nil {
		audio.acquired.release()
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return "", false
	}
	splitBackendCannotServe := audio.lane == recipes.KindMusic || !clusterModelSupportsLlamaAudioPath(audio.acquired.model, r.URL.Path)
	if routeBackendMode == BackendModeLlamaSDCPP && splitBackendCannotServe {
		audio.acquired.release()
		openai.WriteError(w, http.StatusNotImplemented, "unsupported_backend", "audio route is not supported by the selected split backend config")
		return "", false
	}
	return routeBackendMode, true
}

func (service *Service) forwardLocalRegistryAudioRequest(r *http.Request, audio registryAudioRequest, routeBackendMode string) (*http.Response, forwardAnalytics, error) {
	route := audio.acquired.route
	tracking := forwardAnalytics{analytics: service.analytics}
	tracking.event = service.analytics.newEvent(time.Now(), r, audio.requestBody, audio.backendModelID, audioAnalyticsSection(audio.lane), routeBackendMode)
	forwardModelID := route.PublicID
	if routeBackendMode == BackendModeVLLM {
		forwardModelID = audio.backendModelID
	}
	readiness := audioReadiness(r.URL.Path, audio.lane, routeBackendMode)
	target := backendForwardTarget{modelID: forwardModelID, configFilename: route.Filename, hasModel: true, readiness: readiness, mode: routeBackendMode}
	response, finalizer, err := service.forwardWithFallbackObserved(r.Context(), r, audio.requestBody, target)
	tracking.finalizer = finalizer
	return response, tracking, err
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
