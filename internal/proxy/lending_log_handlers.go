package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/openai"
)

const nodeLendingDecisionsPath = "/router/v1/node/offload/decisions"

type lendingDecisionsResponse struct {
	Records    []offloaddecisions.Record `json:"records"`
	NodeErrors []loadCaptureNodeError    `json:"node_errors,omitempty"`
}

func (service *Service) handleSiteLendingDecisions(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	filter, err := parseDecisionFilter(r.URL.Query())
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	selected, err := service.selectedLoadCaptureNodes(r.URL.Query()["node_id"])
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response := lendingDecisionsResponse{Records: []offloaddecisions.Record{}}
	forwarded := cloneQueryWithout(r.URL.Query(), "node_id")
	for _, node := range selected {
		records, requestErr := service.lendingDecisionsFromNode(r.Context(), node, filter, forwarded)
		if requestErr != nil {
			response.NodeErrors = append(response.NodeErrors, loadCaptureNodeError{NodeID: node.NodeID, Error: requestErr.Error()})
			continue
		}
		response.Records = append(response.Records, records...)
	}
	sort.SliceStable(response.Records, func(left, right int) bool {
		return response.Records[left].RecordedAt.After(response.Records[right].RecordedAt)
	})
	if len(response.Records) > filter.Limit {
		response.Records = response.Records[:filter.Limit]
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeLendingDecisions(w http.ResponseWriter, r *http.Request) {
	filter, err := parseDecisionFilter(r.URL.Query())
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	records, err := service.offloadDecisions.Query(r.Context(), filter)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, lendingDecisionsResponse{Records: records})
}

func (service *Service) lendingDecisionsFromNode(ctx context.Context, node loadCaptureNodeTarget, filter offloaddecisions.Filter, forwarded url.Values) ([]offloaddecisions.Record, error) {
	if node.NodeID == service.nodeID {
		return service.offloadDecisions.Query(ctx, filter)
	}
	var response lendingDecisionsResponse
	err := service.clusterClient.JSON(ctx, http.MethodGet, node.URL, nodeLendingDecisionsPath+"?"+forwarded.Encode(), nil, &response)
	return response.Records, err
}

func parseDecisionFilter(values url.Values) (offloaddecisions.Filter, error) {
	filter := offloaddecisions.Filter{
		Lane:    strings.TrimSpace(values.Get("lane")),
		Outcome: strings.TrimSpace(values.Get("outcome")),
		Limit:   500,
	}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 {
			return offloaddecisions.Filter{}, fmt.Errorf("limit must be a positive whole number")
		}
		filter.Limit = limit
	}
	if raw := strings.TrimSpace(values.Get("since_ms")); raw != "" {
		sinceMS, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || sinceMS < 0 {
			return offloaddecisions.Filter{}, fmt.Errorf("since_ms must be a non-negative millisecond timestamp")
		}
		filter.Since = time.UnixMilli(sinceMS)
	}
	return filter, nil
}
