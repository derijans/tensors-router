package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tensors-router/internal/auth"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/mcp"
	"tensors-router/internal/openai"
)

const mcpClusterRequestTimeout = 3 * time.Minute

func (service *Service) handlePublicMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		openai.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	if !auth.PrincipalFromContext(r.Context()).Admin {
		openai.WriteError(w, http.StatusForbidden, "forbidden", "MCP requires an authenticated admin principal")
		return
	}
	modelID := r.URL.Query().Get("model")
	if modelID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "model query parameter is required")
		return
	}
	if service.registry != nil {
		service.handleClusterMCP(w, r, modelID)
		return
	}
	model, ok, err := service.catalog.Resolve(modelID)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	if !ok || !model.MCPEnabled {
		openai.WriteError(w, http.StatusNotFound, "not_found", fmt.Sprintf("MCP model %q was not found", modelID))
		return
	}
	service.handleLocalMCP(w, r, model)
}

func (service *Service) handleClusterMCP(w http.ResponseWriter, r *http.Request, publicID string) {
	model, ok := service.registry.MCPModel(publicID)
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", fmt.Sprintf("MCP model %q was not found", publicID))
		return
	}
	modelBackendMode, err := service.clusterModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	route, release, ok := service.registry.AcquireMCP(publicID, service.localBackendAvailableForRoute(r.Context(), modelBackendMode, readinessText))
	if !ok {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", fmt.Sprintf("MCP model %q has no available replicas", publicID))
		return
	}
	defer release()
	if route.Remote {
		service.forwardRemoteMCP(w, r, route)
		return
	}
	localModel, ok, err := service.catalog.Resolve(route.LocalID)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	if !ok || !localModel.MCPEnabled {
		openai.WriteError(w, http.StatusNotFound, "not_found", fmt.Sprintf("MCP model %q was not found", publicID))
		return
	}
	service.handleLocalMCP(w, r, localModel)
}

func (service *Service) handleNodeMCP(w http.ResponseWriter, r *http.Request) {
	if service.mcpGateway == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	modelID := r.URL.Query().Get("model")
	if modelID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "model query parameter is required")
		return
	}
	model, ok, err := service.catalog.Resolve(modelID)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "catalog_error", err.Error())
		return
	}
	if !ok || !model.MCPEnabled {
		openai.WriteError(w, http.StatusNotFound, "not_found", fmt.Sprintf("MCP model %q was not found", modelID))
		return
	}
	service.handleLocalMCP(w, r, model)
}

func (service *Service) handleLocalMCP(w http.ResponseWriter, r *http.Request, model catalog.Model) {
	enabled, err := service.localModelEnabled(r.Context(), model.ID)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "model_state_error", err.Error())
		return
	}
	if !enabled {
		openai.WriteError(w, http.StatusNotFound, "not_found", fmt.Sprintf("MCP model %q was not found", model.ID))
		return
	}
	backendMode, err := service.catalogModelBackendMode(model)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	modelContext, cancel := context.WithTimeout(r.Context(), modelOperationTimeout)
	defer cancel()
	runtime, release, _, err := service.acquireExactModelConfigForBackendMode(backendMode, modelContext, model.ID, model.Filename, readinessText)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
		return
	}
	defer release()
	if err := service.mcpGateway.ServeHTTP(w, r, mcp.Target{Backend: backendMode, URL: runtime.backend.URL()}); err != nil {
		openai.WriteError(w, http.StatusBadGateway, "backend_error", err.Error())
	}
}

func (service *Service) forwardRemoteMCP(w http.ResponseWriter, r *http.Request, route cluster.Route) {
	content, err := io.ReadAll(r.Body)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	baseURL, err := service.clusterClient.AuthorizedBaseURL(route.NodeURL)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "cluster_error", err.Error())
		return
	}
	target, err := url.Parse(baseURL)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "cluster_error", err.Error())
		return
	}
	target.Path = joinPath(target.Path, "/router/v1/node/mcp")
	query := target.Query()
	query.Set("model", route.LocalID)
	target.RawQuery = query.Encode()
	requestContext, cancel := context.WithTimeout(r.Context(), mcpClusterRequestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, target.String(), bytes.NewReader(content))
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "cluster_error", err.Error())
		return
	}
	copyBackendHeaders(request.Header, r.Header)
	if protocolVersion := strings.TrimSpace(r.Header.Get("Mcp-Protocol-Version")); protocolVersion != "" {
		request.Header.Set("Mcp-Protocol-Version", protocolVersion)
	}
	request.Header.Set("Authorization", "Bearer "+service.clusterToken)
	request.Host = target.Host
	client := *service.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return fmt.Errorf("cluster redirects are not allowed")
	}
	response, err := client.Do(request)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "cluster_error", err.Error())
		return
	}
	defer response.Body.Close()
	if response.ContentLength > service.maxControlBodyBytes {
		openai.WriteError(w, http.StatusBadGateway, "cluster_error", "cluster MCP response is too large")
		return
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, service.maxControlBodyBytes+1))
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "cluster_error", err.Error())
		return
	}
	if int64(len(responseBody)) > service.maxControlBodyBytes {
		openai.WriteError(w, http.StatusBadGateway, "cluster_error", "cluster MCP response is too large")
		return
	}
	copyResponseHeaders(w.Header(), response.Header)
	w.Header().Del("Content-Length")
	w.WriteHeader(response.StatusCode)
	if _, err := w.Write(responseBody); err != nil {
		service.logger.Printf("cluster MCP response failed: %v", err)
	}
}
