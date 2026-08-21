package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

// Launch options are operator-selected environment switches for the vLLM runtime. They
// are read and written through the same site/node split as backend initialization, so a
// master forwards to the owning node instead of answering for it.

func (service *Service) handleSiteBackendLaunchOptions(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	request, ok := decodeBackendLaunchOptionsRequest(w, r)
	if !ok {
		return
	}
	options, err := service.backendLaunchOptions(r.Context(), request, r.Method == http.MethodPost)
	if err != nil {
		writeBackendInitializationError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, options)
}

func (service *Service) handleNodeBackendLaunchOptions(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeBackendLaunchOptionsRequest(w, r)
	if !ok {
		return
	}
	if request.NodeID != service.nodeID {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id does not match this node")
		return
	}
	options, err := service.localBackendLaunchOptions(r.Context(), request, r.Method == http.MethodPost)
	if err != nil {
		writeBackendInitializationError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, options)
}

func (service *Service) backendLaunchOptions(ctx context.Context, request siteapi.BackendLaunchOptionsRequest, apply bool) (siteapi.BackendLaunchOptions, error) {
	if request.NodeID == service.nodeID {
		return service.localBackendLaunchOptions(ctx, request, apply)
	}
	nodeURL, ok := service.remoteNodeURL(request.NodeID)
	if !ok {
		return siteapi.BackendLaunchOptions{}, fmt.Errorf("unknown node %q", request.NodeID)
	}
	method := http.MethodGet
	if apply {
		method = http.MethodPost
	}
	var options siteapi.BackendLaunchOptions
	err := service.clusterClient.JSON(ctx, method, nodeURL, "/router/v1/node/backends/launch-options", request, &options)
	return options, err
}

func (service *Service) localBackendLaunchOptions(ctx context.Context, request siteapi.BackendLaunchOptionsRequest, apply bool) (siteapi.BackendLaunchOptions, error) {
	if service.vllm == nil {
		return siteapi.BackendLaunchOptions{}, fmt.Errorf("vllm companion is unavailable")
	}
	if apply {
		return service.vllm.SetLaunchOptions(ctx, request.Options)
	}
	return service.vllm.LaunchOptions(ctx)
}

func decodeBackendLaunchOptionsRequest(w http.ResponseWriter, r *http.Request) (siteapi.BackendLaunchOptionsRequest, bool) {
	defer r.Body.Close()
	if r.Method == http.MethodGet {
		request := siteapi.BackendLaunchOptionsRequest{NodeID: r.URL.Query().Get("node_id"), BackendID: r.URL.Query().Get("backend_id")}
		if request.NodeID == "" {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id is required")
			return siteapi.BackendLaunchOptionsRequest{}, false
		}
		return request, true
	}
	var request siteapi.BackendLaunchOptionsRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return siteapi.BackendLaunchOptionsRequest{}, false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "request body must contain one JSON object")
		return siteapi.BackendLaunchOptionsRequest{}, false
	}
	if request.NodeID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id is required")
		return siteapi.BackendLaunchOptionsRequest{}, false
	}
	return request, true
}
