package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"tensors-router/internal/openai"
	"tensors-router/internal/proxy/routing"
	"tensors-router/internal/siteapi"
)

const (
	siteMemoryFitPath = "/router/v1/site/nodes/memory-fit"
	nodeMemoryFitPath = "/router/v1/node/memory-fit"
)

func (service *Service) memoryFitRoutes() []routing.Route {
	return append(
		[]routing.Route{routing.Exact(http.MethodGet, siteMemoryFitPath, service.handleSiteMemoryFit)},
		routing.ClusterOnly([]routing.Route{routing.Exact(http.MethodGet, nodeMemoryFitPath, service.handleNodeMemoryFit)})...,
	)
}

func (service *Service) handleSiteMemoryFit(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
	if nodeID == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "node_id is required")
		return
	}
	fit, err := service.nodeMemoryFit(r.Context(), nodeID)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "node_unavailable", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, fit)
}

func (service *Service) handleNodeMemoryFit(w http.ResponseWriter, r *http.Request) {
	openai.WriteJSON(w, http.StatusOK, service.memoryFit.nodeVerdicts(r.Context(), service.nodeID))
}

func (service *Service) nodeMemoryFit(ctx context.Context, nodeID string) (siteapi.NodeMemoryFit, error) {
	if nodeID == service.nodeID {
		return service.memoryFit.nodeVerdicts(ctx, service.nodeID), nil
	}
	nodeURL, ok := service.remoteNodeURL(nodeID)
	if !ok {
		return siteapi.NodeMemoryFit{}, fmt.Errorf("unknown node %q", nodeID)
	}
	var fit siteapi.NodeMemoryFit
	if err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, nodeMemoryFitPath, nil, &fit); err != nil {
		return siteapi.NodeMemoryFit{}, err
	}
	if fit.NodeID != nodeID {
		return siteapi.NodeMemoryFit{}, fmt.Errorf("node %q returned memory fit for %q", nodeID, fit.NodeID)
	}
	return fit, nil
}
