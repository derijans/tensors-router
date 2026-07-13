package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/recipes"
)

func (service *Service) handleRecipeModelRequest(w http.ResponseWriter, r *http.Request, body []byte, publicID string) bool {
	recipe, component, ok := service.recipeModelComponent(publicID, r.URL.Path)
	if !ok {
		return false
	}
	route := routeFromRecipeComponent(recipe, component, false, cluster.RouteLaneText)
	profile := service.localChatTemplateProfile(component.ConfigFilename, component.NodeID != service.nodeID)
	requestBody, transformErr := transformBufferedTransportRequestBody(r, body, component.ModelID, readinessText, profile, true)
	if transformErr != nil {
		writeTransportError(w, transformErr)
		return true
	}
	var response *http.Response
	var err error
	var analyticsEvent routeranalytics.Event
	var workFinalizer analyticsEventFinalizer
	recordAnalytics := false
	if component.NodeID != service.nodeID {
		route.Remote = true
		response, err = service.forwardRemote(r.Context(), r, requestBody, route)
	} else {
		backendMode, modeErr := service.recipeComponentBackendMode(component)
		if modeErr != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", modeErr.Error())
			return true
		}
		started := time.Now()
		analyticsEvent = service.newAnalyticsEvent(started, r, requestBody, component.ModelID, textAnalyticsSection(r.URL.Path), backendMode)
		recordAnalytics = true
		response, workFinalizer, err = service.forwardWithFallbackObserved(r.Context(), r, requestBody, component.ModelID, component.ConfigFilename, true, readinessText, backendMode)
	}
	if err != nil {
		if recordAnalytics {
			service.recordAnalyticsFailure(analyticsEvent, http.StatusBadGateway, workFinalizer)
		}
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return true
	}
	if recordAnalytics {
		response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)
	}
	if err := service.writeModelProxyResponse(w, response, publicID, true); err != nil {
		return true
	}
	return true
}

func (service *Service) handleRecipeImageRequest(w http.ResponseWriter, r *http.Request, body []byte, publicImageID string) bool {
	recipe, component, ok := service.recipeImageComponent(publicImageID)
	if !ok {
		return false
	}
	route := routeFromRecipeComponent(recipe, component, false, cluster.RouteLaneImage)
	request := rewriteImageRequest(r, publicImageID, component.ImageID)
	requestBody := rewriteImageRequestBody(body, publicImageID, component.ImageID, r)
	var response *http.Response
	var err error
	var analyticsEvent routeranalytics.Event
	var workFinalizer analyticsEventFinalizer
	recordAnalytics := false
	jobBackendMode := ""
	if component.NodeID != service.nodeID {
		route.Remote = true
		response, err = service.forwardRemote(r.Context(), request, requestBody, route)
	} else {
		backendMode, modeErr := service.recipeComponentBackendMode(component)
		if modeErr != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", modeErr.Error())
			return true
		}
		jobBackendMode = backendMode
		if backendMode == BackendModeLlamaSDCPP && component.ModelID != "" {
			if err := service.loadLocalRuntimeForRequest(r.Context(), backendMode, component.ModelID, component.ConfigFilename, readinessText); err != nil {
				openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
				return true
			}
		}
		started := time.Now()
		analyticsEvent = service.newAnalyticsEvent(started, request, requestBody, component.ImageID, routeranalytics.SectionImage, backendMode)
		recordAnalytics = true
		response, workFinalizer, err = service.forwardWithFallbackObserved(r.Context(), request, requestBody, component.ImageID, component.ConfigFilename, true, readinessImage, backendMode)
	}
	if err != nil {
		if recordAnalytics {
			service.recordAnalyticsFailure(analyticsEvent, http.StatusBadGateway, workFinalizer)
		}
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return true
	}
	if isSdcppJobSubmissionPath(r.URL.Path) {
		response = service.responseWithSdcppJobTracking(response, sdcppJobTarget{
			publicImageID:  publicImageID,
			configFilename: component.ConfigFilename,
			backendMode:    jobBackendMode,
			remote:         route.Remote,
			nodeURL:        route.NodeURL,
		})
	}
	if recordAnalytics {
		response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)
	}
	if err := service.writeProxyResponse(w, response, publicImageID, true); err != nil {
		return true
	}
	return true
}

func (service *Service) loadRecipe(ctx context.Context, publicID string) (bool, error) {
	if service.recipeStore == nil {
		return false, nil
	}
	recipe, text, hasText := service.recipeStore.Text(publicID)
	_, embeddings, hasEmbeddings := service.recipeStore.Embeddings(publicID)
	_, image, hasImage := service.recipeStore.Image(recipe.PublicImageID)
	_, voice, hasVoice := service.recipeStore.Voice(publicID)
	_, music, hasMusic := service.recipeStore.Music(publicID)
	if !hasText && !hasEmbeddings && !hasImage && !hasVoice && !hasMusic {
		return false, nil
	}
	if hasText {
		if err := service.loadRecipeComponent(ctx, recipe, text, readinessText); err != nil {
			return true, err
		}
	}
	if hasEmbeddings && !sameRecipeComponent(text, embeddings) {
		if err := service.loadRecipeComponent(ctx, recipe, embeddings, readinessText); err != nil {
			return true, err
		}
	}
	if hasImage {
		if err := service.loadRecipeComponent(ctx, recipe, image, readinessImage); err != nil {
			return true, err
		}
	}
	if hasVoice && !sameRecipeComponent(text, voice) && !sameRecipeComponent(embeddings, voice) {
		if err := service.loadRecipeComponent(ctx, recipe, voice, readinessText); err != nil {
			return true, err
		}
	}
	if hasMusic && !sameRecipeComponent(text, music) && !sameRecipeComponent(embeddings, music) && !sameRecipeComponent(voice, music) {
		if err := service.loadRecipeComponent(ctx, recipe, music, readinessText); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (service *Service) loadRecipeComponent(ctx context.Context, recipe recipes.Recipe, component recipes.Component, readiness backendReadiness) error {
	modelID := component.ModelID
	if readiness == readinessImage {
		modelID = component.ImageID
	}
	route := routeFromRecipeComponent(recipe, component, component.NodeID != service.nodeID, routeLaneForReadiness(readiness))
	if route.Remote {
		return service.clusterClient.Load(ctx, route.NodeURL, modelID)
	}
	backendMode, err := service.recipeComponentBackendMode(component)
	if err != nil {
		return err
	}
	return service.loadLocalConfig(ctx, backendMode, modelID, component.ConfigFilename, readiness)
}

func (service *Service) recipeModelComponent(publicID string, path string) (recipes.Recipe, recipes.Component, bool) {
	if service.recipeStore == nil {
		return recipes.Recipe{}, recipes.Component{}, false
	}
	if isEmbeddingsPath(path) {
		if recipe, component, ok := service.recipeStore.Embeddings(publicID); ok {
			return recipe, component, true
		}
	}
	recipe, component, ok := service.recipeStore.Text(publicID)
	return recipe, component, ok
}

func (service *Service) recipeImageComponent(publicImageID string) (recipes.Recipe, recipes.Component, bool) {
	if service.recipeStore == nil {
		return recipes.Recipe{}, recipes.Component{}, false
	}
	return service.recipeStore.Image(publicImageID)
}

func (service *Service) handleRecipeAudioRequest(w http.ResponseWriter, r *http.Request, body []byte, publicID string, lane string) bool {
	recipe, component, ok := service.recipeAudioComponent(publicID, lane)
	if !ok {
		return false
	}
	route := routeFromRecipeComponent(recipe, component, false, audioClusterLane(lane))
	requestBody := body
	if requestBodyLooksJSON(body, r) {
		requestBody = rewriteRequestModel(body, component.ModelID)
	}
	var response *http.Response
	var err error
	var analyticsEvent routeranalytics.Event
	var workFinalizer analyticsEventFinalizer
	recordAnalytics := false
	if component.NodeID != service.nodeID {
		route.Remote = true
		response, err = service.forwardRemote(r.Context(), r, requestBody, route)
	} else {
		backendMode, modeErr := service.recipeComponentBackendMode(component)
		if modeErr != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", modeErr.Error())
			return true
		}
		if backendMode == BackendModeLlamaSDCPP {
			model, ok, modelErr := service.recipeComponentModel(component)
			if modelErr != nil {
				openai.WriteError(w, http.StatusInternalServerError, "catalog_error", modelErr.Error())
				return true
			}
			if !ok || lane == recipes.KindMusic || !modelSupportsLlamaAudioPath(model, r.URL.Path) {
				openai.WriteError(w, http.StatusNotImplemented, "unsupported_backend", "audio route is not supported by the selected split backend config")
				return true
			}
		}
		started := time.Now()
		analyticsEvent = service.newAnalyticsEvent(started, r, requestBody, component.ModelID, audioAnalyticsSection(lane), backendMode)
		recordAnalytics = true
		response, workFinalizer, err = service.forwardWithFallbackObserved(r.Context(), r, requestBody, component.ModelID, component.ConfigFilename, true, readinessText, backendMode)
	}
	if err != nil {
		if recordAnalytics {
			service.recordAnalyticsFailure(analyticsEvent, http.StatusBadGateway, workFinalizer)
		}
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return true
	}
	if recordAnalytics {
		response = service.responseWithAnalytics(response, analyticsEvent, workFinalizer)
	}
	if err := service.writeProxyResponse(w, response, publicID, false); err != nil {
		return true
	}
	return true
}

func (service *Service) recipeAudioComponent(publicID string, lane string) (recipes.Recipe, recipes.Component, bool) {
	if service.recipeStore == nil {
		return recipes.Recipe{}, recipes.Component{}, false
	}
	if lane == recipes.KindMusic {
		return service.recipeStore.Music(publicID)
	}
	return service.recipeStore.Voice(publicID)
}

func routeFromRecipeComponent(recipe recipes.Recipe, component recipes.Component, remote bool, lane string) cluster.Route {
	return cluster.Route{
		PublicID:      recipe.PublicID,
		LocalID:       component.ModelID,
		PublicImageID: recipe.PublicImageID,
		LocalImageID:  component.ImageID,
		Filename:      component.ConfigFilename,
		NodeID:        component.NodeID,
		NodeURL:       component.NodeURL,
		Remote:        remote,
		Lane:          lane,
	}
}

func (service *Service) recipeComponentBackendMode(component recipes.Component) (string, error) {
	models, err := service.catalog.List()
	if err != nil {
		return "", err
	}
	modelID := component.ModelID
	if component.Kind == recipes.KindImage && strings.TrimSpace(component.ImageID) != "" {
		modelID = component.ImageID
	}
	for _, model := range models {
		if model.Filename != component.ConfigFilename {
			continue
		}
		if model.ID == modelID || model.ImageID == modelID || strings.TrimSpace(modelID) == "" {
			return service.catalogModelBackendMode(model)
		}
	}
	return service.resolveBackendMode("")
}

func (service *Service) recipeComponentModel(component recipes.Component) (catalog.Model, bool, error) {
	models, err := service.catalog.List()
	if err != nil {
		return catalog.Model{}, false, err
	}
	modelID := component.ModelID
	if component.Kind == recipes.KindImage && strings.TrimSpace(component.ImageID) != "" {
		modelID = component.ImageID
	}
	for _, model := range models {
		if model.Filename != component.ConfigFilename {
			continue
		}
		if model.ID == modelID || model.ImageID == modelID || strings.TrimSpace(modelID) == "" {
			return model, true, nil
		}
	}
	return catalog.Model{}, false, nil
}

func routeLaneForReadiness(readiness backendReadiness) string {
	if readiness == readinessImage {
		return cluster.RouteLaneImage
	}
	return cluster.RouteLaneText
}

func audioClusterLane(lane string) string {
	if lane == recipes.KindMusic {
		return cluster.RouteLaneMusic
	}
	return cluster.RouteLaneVoice
}

func sameRecipeComponent(left recipes.Component, right recipes.Component) bool {
	return left.NodeID == right.NodeID && left.ConfigFilename == right.ConfigFilename && left.ModelID == right.ModelID
}

func recipeMissingError(publicID string) error {
	return fmt.Errorf("recipe %q was not found", publicID)
}
