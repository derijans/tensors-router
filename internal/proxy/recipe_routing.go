package proxy

import (
	"context"
	"fmt"
	"net/http"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/recipes"
)

func (service *Service) handleRecipeModelRequest(w http.ResponseWriter, r *http.Request, body []byte, publicID string) bool {
	recipe, component, ok := service.recipeModelComponent(publicID, r.URL.Path)
	if !ok {
		return false
	}
	requestBody := rewriteRequestModel(body, component.ModelID)
	route := routeFromRecipeComponent(recipe, component, false, cluster.RouteLaneText)
	var response *http.Response
	var err error
	if component.NodeID != service.nodeID {
		route.Remote = true
		response, err = service.forwardRemote(r.Context(), r, requestBody, route)
	} else {
		response, err = service.forwardWithFallback(r.Context(), r, requestBody, component.ModelID, component.ConfigFilename, true, readinessText)
	}
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return true
	}
	if err := writeModelProxyResponse(w, response, publicID, true); err != nil {
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
	if component.NodeID != service.nodeID {
		route.Remote = true
		response, err = service.forwardRemote(r.Context(), request, requestBody, route)
	} else {
		response, err = service.forwardWithFallback(r.Context(), request, requestBody, component.ImageID, component.ConfigFilename, true, readinessImage)
	}
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return true
	}
	if err := writeProxyResponse(w, response, publicImageID, true); err != nil {
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
	route := routeFromRecipeComponent(recipe, component, component.NodeID != service.nodeID, routeLaneForReadiness(readiness))
	if route.Remote {
		return service.clusterClient.Load(ctx, route.NodeURL, route.LocalID)
	}
	runtime := service.runtimeForReadiness(readiness)
	modelID := component.ModelID
	if readiness == readinessImage {
		modelID = component.ImageID
	}
	return service.loadLocalConfig(ctx, runtime, modelID, component.ConfigFilename, readiness)
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
	if component.NodeID != service.nodeID {
		route.Remote = true
		response, err = service.forwardRemote(r.Context(), r, requestBody, route)
	} else {
		response, err = service.forwardWithFallback(r.Context(), r, requestBody, component.ModelID, component.ConfigFilename, true, readinessText)
	}
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return true
	}
	if err := writeProxyResponse(w, response, publicID, false); err != nil {
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
