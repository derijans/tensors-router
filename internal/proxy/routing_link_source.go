package proxy

import (
	"context"
	"encoding/json"
	"net/http"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/routinggroups"
)

func (service *Service) routingLinkIndex() *routingLinkIndex {
	return service.routingLinks.Load()
}

func (service *Service) installRoutingLinks(snapshot routingLinkSnapshot) {
	service.routingLinks.Store(newRoutingLinkIndex(snapshot))
}

func (service *Service) storedRoutingLinks(ctx context.Context) (routingLinkSnapshot, error) {
	imageLinks, err := service.routingGroups.Links(ctx, routinggroups.ImageLane)
	if err != nil {
		return routingLinkSnapshot{}, err
	}
	textLinks, err := service.routingGroups.Links(ctx, routinggroups.TextLane)
	if err != nil {
		return routingLinkSnapshot{}, err
	}
	return routingLinkSnapshot{Image: imageLinks, Text: textLinks}, nil
}

func (service *Service) installStoredRoutingLinks(ctx context.Context) (routingLinkSnapshot, bool) {
	if service.routingGroups == nil {
		return routingLinkSnapshot{}, false
	}
	snapshot, err := service.storedRoutingLinks(ctx)
	if err != nil {
		service.logger.Printf("routing link load failed: %v", err)
		return routingLinkSnapshot{}, false
	}
	service.installRoutingLinks(snapshot)
	return snapshot, true
}

func (service *Service) installAndPublishStoredRoutingLinks(ctx context.Context) {
	snapshot, loaded := service.installStoredRoutingLinks(ctx)
	if loaded {
		service.publishRoutingLinks(ctx, snapshot)
	}
}

func (service *Service) publishRoutingLinks(ctx context.Context, snapshot routingLinkSnapshot) {
	if service.clusterRole != cluster.RoleMaster || service.registry == nil {
		return
	}
	for nodeID, nodeURL := range service.registry.NodeURLsByID() {
		if nodeID == service.nodeID || nodeURL == "" {
			continue
		}
		if err := service.clusterClient.JSON(ctx, http.MethodPost, nodeURL, nodeRoutingLinksPath, snapshot, nil); err != nil {
			service.logger.Printf("routing link delivery failed node=%s error=%v", nodeID, err)
		}
	}
}

const nodeRoutingLinksPath = "/router/v1/node/routing-links"

func (service *Service) handleNodeRoutingLinks(w http.ResponseWriter, r *http.Request) {
	if service.clusterRole != cluster.RoleSlave {
		openai.WriteError(w, http.StatusConflict, "invalid_request_error", "only a slave accepts routing links from its master")
		return
	}
	var snapshot routingLinkSnapshot
	if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	service.installRoutingLinks(snapshot)
	w.WriteHeader(http.StatusNoContent)
}
