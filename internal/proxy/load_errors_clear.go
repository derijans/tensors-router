package proxy

import (
	"context"
	"net/http"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
	"tensors-router/internal/proxy/clusterfan"
)

const nodeLoadErrorsPath = "/router/v1/node/load-errors"

type loadErrorClearResponse struct {
	Cleared      int64                  `json:"cleared"`
	ClearedNodes []string               `json:"cleared_nodes"`
	NodeErrors   []loadCaptureNodeError `json:"node_errors,omitempty"`
}

func (service *Service) handleSiteLoadErrorsClear(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	response := service.clearLocalLoadErrors(r.Context())
	if service.clusterRole == cluster.RoleMaster {
		results := clusterfan.Nodes(r.Context(), service.remoteInventoryURLs(), func(nodeContext context.Context, nodeURL string) (loadErrorClearResponse, error) {
			var remote loadErrorClearResponse
			err := service.clusterClient.JSON(nodeContext, http.MethodDelete, nodeURL, nodeLoadErrorsPath, nil, &remote)
			return remote, err
		})
		for _, result := range results {
			if result.Err != nil {
				response.NodeErrors = append(response.NodeErrors, loadCaptureNodeError{NodeID: service.nodeIDForURL(result.Target), Error: result.Err.Error()})
				continue
			}
			response.Cleared += result.Value.Cleared
			response.ClearedNodes = append(response.ClearedNodes, result.Value.ClearedNodes...)
			response.NodeErrors = append(response.NodeErrors, result.Value.NodeErrors...)
		}
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeLoadErrorsClear(w http.ResponseWriter, r *http.Request) {
	openai.WriteJSON(w, http.StatusOK, service.clearLocalLoadErrors(r.Context()))
}

func (service *Service) clearLocalLoadErrors(ctx context.Context) loadErrorClearResponse {
	response := loadErrorClearResponse{ClearedNodes: []string{}}
	if service.loadErrorStore == nil {
		return response
	}
	cleared, err := service.loadErrorStore.Clear(ctx)
	if err != nil {
		response.NodeErrors = append(response.NodeErrors, loadCaptureNodeError{NodeID: service.nodeID, Error: err.Error()})
		return response
	}
	response.Cleared = cleared
	response.ClearedNodes = append(response.ClearedNodes, service.nodeID)
	return response
}
