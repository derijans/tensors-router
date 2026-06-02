package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

func (service *Service) handleRegistryModelRequest(w http.ResponseWriter, r *http.Request, body []byte, publicID string) {
	model, ok := service.registry.Model(publicID)
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", publicID))
		return
	}
	if !registryModelSupportsOpenAIPath(model, r.URL.Path) {
		openai.WriteError(w, http.StatusNotFound, "invalid_request_error", fmt.Sprintf("model %q was not found", publicID))
		return
	}

	route, release, ok := service.registry.Acquire(publicID, service.backend.Healthy(r.Context()))
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
		response, err = service.forwardWithFallback(r.Context(), r, requestBody, route.PublicID, route.Filename, true, readinessText)
	}
	if err != nil {
		release()
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	response = responseWithRelease(response, release)

	if err := writeModelProxyResponse(w, response, publicID, true); err != nil {
		return
	}
}

func (service *Service) forwardRemote(ctx context.Context, original *http.Request, body []byte, route cluster.Route) (*http.Response, error) {
	target, err := url.Parse(route.NodeURL)
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

func modelSupportsOpenAIPath(model catalog.Model, path string) bool {
	if path == "/v1/embeddings" {
		return model.HasEmbeddings || model.HasLLM
	}
	if isCorePath(path) {
		return model.HasLLM
	}
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
