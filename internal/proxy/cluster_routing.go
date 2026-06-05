package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func (service *Service) handleRegistryModelRequest(w http.ResponseWriter, r *http.Request, body []byte, publicID string) {
	model, route, release, ok := service.acquireRegistryModelRoute(r, publicID)
	defer release()
	if !registryModelSupportsOpenAIPath(model, r.URL.Path) {
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", publicID))
		return
	}
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("model %q has no available replicas", publicID))
		return
	}

	requestBody := rewriteRequestModel(body, route.LocalID)
	var response *http.Response
	var err error
	if route.Remote {
		response, err = service.forwardRemote(r.Context(), r, requestBody, route)
	} else {
		if service.backendMode == BackendModeLlamaSDCPP && model.HasImage && !isEmbeddingsPath(r.URL.Path) {
			if err := service.loadLocalRuntimeForRequest(r.Context(), service.imageRuntime, route.PublicImageID, route.Filename, readinessImage); err != nil {
				openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
				return
			}
		}
		response, err = service.forwardWithFallback(r.Context(), r, requestBody, route.PublicID, route.Filename, true, readinessText)
	}
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	response = responseWithRelease(response, release)

	if err := writeModelProxyResponse(w, response, publicID, true); err != nil {
		return
	}
}

func (service *Service) acquireRegistryModelRoute(r *http.Request, publicID string) (cluster.Model, cluster.Route, func(), bool) {
	if isEmbeddingsPath(r.URL.Path) {
		model, ok := service.registry.EmbeddingModel(publicID)
		if !ok {
			return cluster.Model{}, cluster.Route{}, func() {}, false
		}
		route, release, ok := service.registry.AcquireEmbedding(publicID, service.textRuntime.backend.Healthy(r.Context()))
		return model, route, release, ok
	}
	model, ok := service.registry.Model(publicID)
	if !ok {
		return cluster.Model{}, cluster.Route{}, func() {}, false
	}
	route, release, ok := service.registry.Acquire(publicID, service.textRuntime.backend.Healthy(r.Context()))
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

	route, release, ok := service.registry.AcquireImage(publicImageID, service.imageRuntime.backend.Healthy(r.Context()), activeConfigFilename)
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("image model %q has no available replicas", publicImageID))
		return true
	}

	request := rewriteImageRequest(r, publicImageID, route.LocalImageID)
	requestBody := rewriteImageRequestBody(body, publicImageID, route.LocalImageID, r)
	var response *http.Response
	var err error
	if route.Remote {
		response, err = service.forwardRemote(r.Context(), request, requestBody, route)
	} else {
		if service.backendMode == BackendModeLlamaSDCPP && (model.HasLLM || model.HasEmbeddings || model.HasMultimodal) {
			if err := service.loadLocalRuntimeForRequest(r.Context(), service.textRuntime, route.PublicID, route.Filename, readinessText); err != nil {
				release()
				openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
				return true
			}
		}
		response, err = service.forwardWithFallback(r.Context(), request, requestBody, route.PublicImageID, route.Filename, true, readinessImage)
	}
	if err != nil {
		release()
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return true
	}
	response = responseWithRelease(response, release)

	if err := writeProxyResponse(w, response, publicImageID, true); err != nil {
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

	route, release, ok := service.registry.AcquireImage(publicImageID, service.imageRuntime.backend.Healthy(r.Context()), activeConfigFilename)
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
		if err := writeProxyResponse(w, response, publicImageID, true); err != nil {
			return true
		}
		return true
	}

	modelContext, cancelModelContext := context.WithTimeout(context.WithoutCancel(r.Context()), modelOperationTimeout)
	defer cancelModelContext()
	if service.backendMode == BackendModeLlamaSDCPP && (model.HasLLM || model.HasEmbeddings || model.HasMultimodal) {
		if err := service.loadLocalConfig(modelContext, service.textRuntime, route.PublicID, route.Filename, readinessText); err != nil {
			release()
			openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
			return true
		}
	}
	releaseModel, _, err := service.acquireModelConfig(service.imageRuntime, modelContext, route.PublicImageID, route.Filename, readinessImage, false)
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

func (service *Service) forwardRemote(ctx context.Context, original *http.Request, body []byte, route cluster.Route) (*http.Response, error) {
	baseURL, err := service.clusterClient.AuthorizedBaseURL(route.NodeURL)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	target.Path = joinPath(target.Path, original.URL.Path)
	target.RawQuery = original.URL.RawQuery

	request, err := http.NewRequestWithContext(ctx, original.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyRequestHeaders(request.Header, original.Header)
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
	if !model.HasImage || model.PublicImageID == "" {
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
	if path == "/v1/embeddings" {
		return model.HasEmbeddings || model.HasLLM
	}
	if isCorePath(path) {
		return model.HasLLM
	}
	return modelSupportsTextLane(model)
}

func modelSupportsTextLane(model catalog.Model) bool {
	return model.HasLLM || model.HasEmbeddings || model.HasMultimodal
}

func registryModelSupportsOpenAIPath(model cluster.Model, path string) bool {
	if path == "/v1/embeddings" {
		return model.HasEmbeddings || model.HasLLM
	}
	if isCorePath(path) {
		return model.HasLLM
	}
	return model.HasLLM || model.HasEmbeddings || model.HasMultimodal
}
