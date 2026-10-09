package proxy

import (
	"net/http"
	"sort"
	"time"

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

const nodeLendingSummaryPath = "/router/v1/node/offload/summary"

type lendingNodeSummary struct {
	NodeID              string                    `json:"node_id"`
	BuildVersion        string                    `json:"build_version"`
	SettingsFingerprint string                    `json:"settings_fingerprint"`
	HeldRequests        []siteapi.NodeHeldRequest `json:"held_requests"`
}

type lendingSummaryResponse struct {
	Nodes      []lendingNodeSummary   `json:"nodes"`
	Leases     []offloadLease         `json:"leases"`
	NodeErrors []loadCaptureNodeError `json:"node_errors,omitempty"`
}

func (service *Service) handleSiteLendingSummary(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteEndpointNotFound(w)
		return
	}
	selected, err := service.selectedLoadCaptureNodes(nil)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response := lendingSummaryResponse{Nodes: []lendingNodeSummary{}, Leases: service.scheduler.leaseBook.Live(time.Now())}
	for _, node := range selected {
		if node.NodeID == service.nodeID {
			response.Nodes = append(response.Nodes, service.localLendingSummary())
			continue
		}
		var summary lendingNodeSummary
		if err := service.clusterClient.JSON(r.Context(), http.MethodGet, node.URL, nodeLendingSummaryPath, nil, &summary); err != nil {
			response.NodeErrors = append(response.NodeErrors, loadCaptureNodeError{NodeID: node.NodeID, Error: err.Error()})
			continue
		}
		response.Nodes = append(response.Nodes, summary)
	}
	sort.Slice(response.Nodes, func(left, right int) bool { return response.Nodes[left].NodeID < response.Nodes[right].NodeID })
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeLendingSummary(w http.ResponseWriter, _ *http.Request) {
	openai.WriteJSON(w, http.StatusOK, service.localLendingSummary())
}

func (service *Service) localLendingSummary() lendingNodeSummary {
	return lendingNodeSummary{
		NodeID:              service.nodeID,
		BuildVersion:        buildinfo.Current().Version,
		SettingsFingerprint: service.scheduler.currentSettings().Fingerprint(),
		HeldRequests:        service.scheduler.heldRequests(time.Now()),
	}
}
