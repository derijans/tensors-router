package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"tensors-router/internal/backendmode"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/recipes"
)

func borrowedRequestMustStayOnThisNode(r *http.Request, route cluster.Route) bool {
	return requestIsBorrowed(r) && route.Remote
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

func clusterModelSupportsLlamaAudioPath(model cluster.Model, path string) bool {
	if model.Capabilities.Voice == nil {
		return false
	}
	switch path {
	case "/v1/audio/speech":
		return strings.TrimSpace(model.Capabilities.Voice.TalkerModel) != ""
	case pathAudioTranscriptions:
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
	target.Path = joinPath(target.Path, nodeInferencePrefix+original.URL.Path)
	target.RawQuery = original.URL.RawQuery

	request, err := http.NewRequestWithContext(ctx, original.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyClusterRequestHeaders(request.Header, original.Header)
	request.Header.Set("Authorization", "Bearer "+service.clusterToken)
	request.Host = target.Host
	response, err := service.client.Do(request)
	if err != nil {
		return nil, service.nodeUnreachable(route.NodeID, route.NodeURL, err)
	}
	return response, nil
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
	if strings.TrimSpace(rewritten.Header.Get(headerTensorsModel)) == publicImageID {
		rewritten.Header.Set(headerTensorsModel, localImageID)
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
