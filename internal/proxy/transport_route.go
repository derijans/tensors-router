package proxy

import (
	"fmt"
	"net/http"
	"strings"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/recipes"
	"tensors-router/internal/transportbody"
)

type transportRoute struct {
	publicID       string
	localID        string
	configFilename string
	backendMode    string
	readiness      backendReadiness
	section        string
	remote         bool
	nodeURL        string
	rewriteModel   bool
	insertModel    bool
	passthrough    bool
	release        func()
	catalogModel   catalog.Model
	clusterModel   cluster.Model
}

type transportRouteError struct {
	status  int
	code    string
	message string
}

var errEndpointNotFoundRoute = transportRouteError{http.StatusNotFound, "not_found", "endpoint not found"}

func (err transportRouteError) Error() string {
	return err.message
}

func (service *Service) handleStreamingRequest(w http.ResponseWriter, r *http.Request, body transportbody.Body, selector string) {
	route, err := service.resolveTransportRoute(r, selector)
	if err != nil {
		writeTransportRouteError(w, err)
		return
	}
	if route.release == nil {
		route.release = func() {}
	}
	request := rewriteTransportRequestSelectors(r, route.localID, route.readiness)
	profile := service.localChatTemplateProfile(route.configFilename, route.remote)
	forwardBody, err := transformTransportRequestBody(request, body, route, profile)
	if err != nil {
		route.release()
		writeTransportError(w, err)
		return
	}
	if forwardBody != body {
		defer forwardBody.Close()
	}

	response, event, finalizer, err := service.forwardTransportRoute(request, forwardBody, route)
	event.RequestBytes = forwardBody.BytesConsumed()
	if err != nil {
		route.release()
		if event.ModelID != "" {
			service.analytics.recordForwardFailure(request.Context(), event, err, finalizer)
		}
		service.writeTransportForwardError(w, err)
		return
	}
	if isSdcppJobSubmissionPath(request.URL.Path) {
		response = service.responseWithSdcppJobTracking(response, sdcppJobTarget{
			publicImageID:  route.publicID,
			configFilename: route.configFilename,
			backendMode:    route.backendMode,
			remote:         route.remote,
			nodeURL:        route.nodeURL,
		})
	}
	if route.backendMode == BackendModeVLLM && request.URL.Path == "/v1/responses" {
		response = service.responseWithVLLMTracking(response, vllmResponseTarget{
			publicID:       route.publicID,
			localID:        route.localID,
			configFilename: route.configFilename,
			remote:         route.remote,
			nodeURL:        route.nodeURL,
		})
	}
	response = responseWithRelease(response, route.release)
	if event.ModelID != "" {
		response = service.analytics.withResponse(response, event, finalizer)
	}
	if err := service.writeProxyResponse(w, response, route.publicID, route.rewriteModel); err != nil {
		return
	}
}

func (service *Service) resolveTransportRoute(r *http.Request, selector string) (transportRoute, error) {
	selector = strings.TrimSpace(selector)
	automaticallySelected := false
	if selector == "" && isEmbeddingsPath(r.URL.Path) {
		if target, selected := service.acquireSelectorlessEmbeddingTarget(r.URL.Path, r.Context()); selected {
			return target.transportRoute(r.URL.Path), nil
		}
		mode, err := service.resolveBackendMode("")
		if err != nil {
			return transportRoute{}, err
		}
		return transportRoute{
			backendMode: mode,
			readiness:   readinessEmbeddings,
			section:     textAnalyticsSection(r.URL.Path),
			passthrough: true,
			release:     func() {},
		}, nil
	}
	if selector == "" && selectorlessVLLMPath(r.URL.Path) {
		var err error
		selector, err = service.selectSelectorlessVLLMModel(r.URL.Path)
		if err != nil {
			return transportRoute{}, err
		}
		automaticallySelected = true
	}
	if selector == "" {
		return transportRoute{}, transportRouteError{http.StatusBadRequest, "streaming_model_selector_required", transportbody.ErrSelectorRequired.Error()}
	}
	var route transportRoute
	var err error
	switch {
	case isVoicePath(r.URL.Path) || isMusicPath(r.URL.Path):
		route, err = service.resolveTransportAudioRoute(r, selector)
	case isImagePath(r.URL.Path):
		route, err = service.resolveTransportImageRoute(r, selector)
	default:
		route, err = service.resolveTransportTextRoute(r, selector)
	}
	if err != nil {
		return transportRoute{}, err
	}
	if route.backendMode == BackendModeVLLM && !vllmInferenceAllowed(r.Method, r.URL.Path) {
		if route.release != nil {
			route.release()
		}
		return transportRoute{}, errEndpointNotFoundRoute
	}
	if automaticallySelected {
		route.rewriteModel = false
	}
	return route, nil
}

func (service *Service) resolveTransportImageRoute(r *http.Request, publicID string) (transportRoute, error) {
	if recipe, component, ok := service.recipeImageComponent(publicID); ok {
		return service.transportRecipeRoute(recipe, component, publicID, component.ImageID, readinessImage, routeranalytics.SectionImage, true)
	}
	activeConfig := service.imageCatalogConfigSelector()
	if service.registry != nil {
		if model, ok := service.registry.ImageModel(publicID, activeConfig); ok {
			mode, err := service.clusterModelBackendMode(model)
			if err != nil {
				return transportRoute{}, err
			}
			route, release, acquired := service.registry.AcquireImage(publicID, service.localBackendAvailableForRoute(r.Context(), mode, readinessImage), activeConfig)
			if !acquired {
				return transportRoute{}, transportRouteError{http.StatusBadGateway, "backend_error", fmt.Sprintf("image model %q has no available replicas", publicID)}
			}
			mode, err = service.clusterRouteBackendMode(route, model)
			if err != nil {
				release()
				return transportRoute{}, err
			}
			return transportRoute{
				publicID:       publicID,
				localID:        route.LocalImageID,
				configFilename: route.Filename,
				backendMode:    mode,
				readiness:      readinessImage,
				section:        routeranalytics.SectionImage,
				remote:         route.Remote,
				nodeURL:        route.NodeURL,
				rewriteModel:   true,
				release:        release,
				clusterModel:   model,
			}, nil
		}
	}
	model, err := service.resolveImageModel(r, publicID)
	if err != nil {
		return transportRoute{}, modelNotFoundRouteError(publicID)
	}
	mode, err := service.catalogModelBackendMode(model)
	if err != nil {
		return transportRoute{}, err
	}
	return transportRoute{
		publicID:       publicID,
		localID:        model.ImageID,
		configFilename: model.Filename,
		backendMode:    mode,
		readiness:      readinessImage,
		section:        routeranalytics.SectionImage,
		rewriteModel:   true,
		catalogModel:   model,
	}, nil
}

func (service *Service) resolveTransportAudioRoute(r *http.Request, publicID string) (transportRoute, error) {
	lane := audioRouteKind(r.URL.Path)
	if recipe, component, ok := service.recipeAudioComponent(publicID, lane); ok {
		mode, err := service.recipeComponentBackendMode(component)
		if err != nil {
			return transportRoute{}, err
		}
		return service.transportRecipeRoute(recipe, component, publicID, component.ModelID, audioReadiness(r.URL.Path, lane, mode), audioAnalyticsSection(lane), false)
	}
	if service.registry != nil && service.registryHasAudioModel(publicID, lane) {
		model, ok := service.registryAudioModel(publicID, lane)
		if !ok {
			return transportRoute{}, modelNotFoundRouteError(publicID)
		}
		mode, err := service.clusterModelBackendMode(model)
		if err != nil {
			return transportRoute{}, err
		}
		readiness := audioReadiness(r.URL.Path, lane, mode)
		route, release, acquired := service.acquireRegistryAudioRoute(r, publicID, lane, mode, readiness)
		if !acquired {
			return transportRoute{}, transportRouteError{http.StatusBadGateway, "backend_error", fmt.Sprintf("model %q has no available replicas", publicID)}
		}
		mode, err = service.resolveBackendMode(route.BackendMode)
		if err != nil {
			release()
			return transportRoute{}, err
		}
		localID := route.LocalID
		if mode == BackendModeVLLM {
			localID = vllmRequestModelID(publicID, route.LocalID, model.ServedNames)
		}
		return transportRoute{
			publicID:       publicID,
			localID:        localID,
			configFilename: route.Filename,
			backendMode:    mode,
			readiness:      audioReadiness(r.URL.Path, lane, mode),
			section:        audioAnalyticsSection(lane),
			remote:         route.Remote,
			nodeURL:        route.NodeURL,
			release:        release,
			clusterModel:   model,
		}, nil
	}
	model, ok, err := service.catalog.Resolve(publicID)
	if err != nil {
		return transportRoute{}, err
	}
	if !ok || !modelSupportsAudioLane(model, lane) {
		return transportRoute{}, modelNotFoundRouteError(publicID)
	}
	mode, err := service.catalogModelBackendMode(model)
	if err != nil {
		return transportRoute{}, err
	}
	localID := model.ID
	if mode == BackendModeVLLM {
		localID = vllmRequestModelID(publicID, model.ID, model.ServedNames)
	}
	return transportRoute{
		publicID:       publicID,
		localID:        localID,
		configFilename: model.Filename,
		backendMode:    mode,
		readiness:      audioReadiness(r.URL.Path, lane, mode),
		section:        audioAnalyticsSection(lane),
		catalogModel:   model,
	}, nil
}

func (service *Service) transportRecipeRoute(recipe recipes.Recipe, component recipes.Component, publicID string, localID string, readiness backendReadiness, section string, rewrite bool) (transportRoute, error) {
	mode := ""
	var err error
	remote := component.NodeID != service.nodeID
	if !remote {
		mode, err = service.recipeComponentBackendMode(component)
		if err != nil {
			return transportRoute{}, err
		}
	}
	model, _, modelErr := service.recipeComponentModel(component)
	if modelErr != nil {
		return transportRoute{}, modelErr
	}
	return transportRoute{
		publicID:       publicID,
		localID:        localID,
		configFilename: component.ConfigFilename,
		backendMode:    mode,
		readiness:      readiness,
		section:        section,
		remote:         remote,
		nodeURL:        component.NodeURL,
		rewriteModel:   rewrite,
		release:        func() {},
		catalogModel:   model,
	}, nil
}

func rewriteTransportRequestSelectors(original *http.Request, localID string, readiness backendReadiness) *http.Request {
	rewritten := original.Clone(original.Context())
	rewritten.Header = original.Header.Clone()
	requestURL := *original.URL
	values := requestURL.Query()
	keys := []string{"model"}
	if readiness == readinessImage {
		keys = append(keys, "sd_model_checkpoint")
	}
	for _, key := range keys {
		if strings.TrimSpace(values.Get(key)) != "" {
			values.Set(key, localID)
		}
	}
	requestURL.RawQuery = values.Encode()
	rewritten.URL = &requestURL
	if strings.TrimSpace(rewritten.Header.Get(headerTensorsModel)) != "" {
		rewritten.Header.Set(headerTensorsModel, localID)
	}
	return rewritten
}

func modelNotFoundRouteError(modelID string) error {
	return transportRouteError{http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", modelID)}
}

func writeTransportRouteError(w http.ResponseWriter, err error) {
	if routeErr, ok := err.(transportRouteError); ok {
		openai.WriteError(w, routeErr.status, routeErr.code, routeErr.message)
		return
	}
	openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
}

func (service *Service) resolveTransportTextRoute(r *http.Request, publicID string) (transportRoute, error) {
	if recipe, component, ok := service.recipeModelComponent(publicID, r.URL.Path); ok {
		return service.transportRecipeRoute(recipe, component, publicID, component.ModelID, modelReadiness(r.URL.Path), textAnalyticsSection(r.URL.Path), true)
	}
	if service.registry != nil && service.registryHasModelForOpenAIPath(publicID, r.URL.Path) {
		return service.resolveRegistryTextRoute(r, publicID)
	}
	return service.resolveCatalogTextRoute(r, publicID)
}

func (service *Service) resolveRegistryTextRoute(r *http.Request, publicID string) (transportRoute, error) {
	model, route, release, ok := service.acquireRegistryModelRoute(r, publicID)
	if !ok {
		return transportRoute{}, transportRouteError{http.StatusBadGateway, "backend_error", fmt.Sprintf("model %q has no available replicas", publicID)}
	}
	if !registryModelSupportsOpenAIPath(model, r.URL.Path) {
		release()
		return transportRoute{}, modelNotFoundRouteError(publicID)
	}
	mode, err := service.clusterRouteBackendMode(route, model)
	if err != nil {
		release()
		return transportRoute{}, err
	}
	target, err := textRouteTarget(r, publicID, mode, route.LocalID, model.ServedNames, model.VLLMTask)
	if err != nil {
		release()
		return transportRoute{}, err
	}
	return transportRoute{
		publicID:       publicID,
		localID:        target.localID,
		configFilename: route.Filename,
		backendMode:    mode,
		readiness:      target.readiness,
		section:        textAnalyticsSection(r.URL.Path),
		remote:         route.Remote,
		nodeURL:        route.NodeURL,
		rewriteModel:   true,
		release:        release,
		clusterModel:   model,
	}, nil
}

func (service *Service) resolveCatalogTextRoute(r *http.Request, publicID string) (transportRoute, error) {
	model, ok, err := service.resolveCatalogModelForOpenAIPath(publicID, r.URL.Path)
	if err != nil {
		return transportRoute{}, err
	}
	if !ok || !modelSupportsOpenAIPath(model, r.URL.Path) {
		return transportRoute{}, modelNotFoundRouteError(publicID)
	}
	mode, err := service.catalogModelBackendMode(model)
	if err != nil {
		return transportRoute{}, err
	}
	target, err := textRouteTarget(r, publicID, mode, model.ID, model.ServedNames, model.VLLMTask)
	if err != nil {
		return transportRoute{}, err
	}
	return transportRoute{
		publicID:       publicID,
		localID:        target.localID,
		configFilename: model.Filename,
		backendMode:    mode,
		readiness:      target.readiness,
		section:        textAnalyticsSection(r.URL.Path),
		rewriteModel:   true,
		catalogModel:   model,
	}, nil
}

type textRouteSelection struct {
	localID   string
	readiness backendReadiness
}

func textRouteTarget(r *http.Request, publicID string, mode string, localID string, servedNames []string, vllmTask string) (textRouteSelection, error) {
	if mode != BackendModeVLLM {
		return textRouteSelection{localID: localID, readiness: modelReadiness(r.URL.Path)}, nil
	}
	if !vllmInferenceAllowed(r.Method, r.URL.Path) {
		return textRouteSelection{}, errEndpointNotFoundRoute
	}
	return textRouteSelection{
		localID:   vllmRequestModelID(publicID, localID, servedNames),
		readiness: vllmReadinessForTask(r.URL.Path, vllmTask),
	}, nil
}
