package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"tensors-router/internal/backenddiagnostic"
	"tensors-router/internal/loaderrors"
	"tensors-router/internal/openai"
)

type loadErrorListResponse struct {
	Enabled    bool                   `json:"enabled"`
	Nodes      []loadCaptureNode      `json:"nodes"`
	Records    []loaderrors.Record    `json:"records"`
	NodeErrors []loadCaptureNodeError `json:"node_errors,omitempty"`
}

func (service *Service) recordLoadError(input loaderrors.RecordInput) {
	if service == nil || service.loadErrorStore == nil {
		return
	}
	if strings.TrimSpace(input.NodeID) == "" {
		input.NodeID = service.nodeID
	}
	if err := service.loadErrorStore.Record(context.Background(), input); err != nil {
		service.logger.Printf("load error store rejected a %s record from %s: %v", input.Phase, input.Source, err)
	}
}

func (service *Service) RecordConfigWarnings(warnings []string) {
	for _, warning := range warnings {
		service.recordLoadError(loaderrors.RecordInput{
			Phase:    loaderrors.PhaseConfigParse,
			Severity: loaderrors.SeverityWarning,
			Source:   "config.load",
			Message:  warning,
		})
	}
}

func (service *Service) recordLoadErrorFromErr(phase loaderrors.Phase, source string, configName string, err error, secrets ...string) {
	if err == nil || service.loadErrorStore == nil {
		return
	}
	input := loaderrors.RecordInput{
		Phase:      phase,
		Severity:   loaderrors.SeverityError,
		Source:     source,
		ConfigName: configName,
		Message:    err.Error(),
		Secrets:    secrets,
	}
	if diagnostic, ok := backenddiagnostic.FromError(err); ok {
		input.Output = diagnostic.Output
		input.ExitError = diagnostic.ExitError
		input.Truncated = diagnostic.Truncated
		if diagnostic.NodeID != "" {
			input.NodeID = diagnostic.NodeID
		}
		if diagnostic.Backend != "" {
			input.Backend = diagnostic.Backend
		}
	}
	service.recordLoadError(input)
}

func (service *Service) handleSiteLoadErrors(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	filter, err := parseLoadErrorQuery(r.URL.Query())
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	selected, err := service.selectedLoadCaptureNodes(r.URL.Query()["node_id"])
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response := loadErrorListResponse{Records: []loaderrors.Record{}, Nodes: []loadCaptureNode{}}
	for _, node := range selected {
		result, requestErr := service.loadErrorsFromNode(r.Context(), node, filter, r.URL.Query())
		if requestErr != nil {
			response.NodeErrors = append(response.NodeErrors, loadCaptureNodeError{NodeID: node.NodeID, Error: requestErr.Error()})
			continue
		}
		response.Nodes = append(response.Nodes, result.Nodes...)
		response.Records = append(response.Records, result.Records...)
		response.Enabled = response.Enabled || result.Enabled
	}
	sort.Slice(response.Nodes, func(left, right int) bool { return response.Nodes[left].NodeID < response.Nodes[right].NodeID })
	sort.Slice(response.Records, func(left, right int) bool {
		if response.Records[left].LastSeenAt.Equal(response.Records[right].LastSeenAt) {
			return response.Records[left].ID > response.Records[right].ID
		}
		return response.Records[left].LastSeenAt.After(response.Records[right].LastSeenAt)
	})
	if filter.Limit > 0 && len(response.Records) > filter.Limit {
		response.Records = response.Records[:filter.Limit]
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) handleNodeLoadErrors(w http.ResponseWriter, r *http.Request) {
	filter, err := parseLoadErrorQuery(r.URL.Query())
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response, err := service.localLoadErrorList(r.Context(), filter)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) loadErrorsFromNode(ctx context.Context, node loadCaptureNodeTarget, filter loaderrors.ListFilter, values url.Values) (loadErrorListResponse, error) {
	if node.NodeID == service.nodeID {
		return service.localLoadErrorList(ctx, filter)
	}
	forwarded := cloneQueryWithout(values, "node_id")
	var response loadErrorListResponse
	err := service.clusterClient.JSON(ctx, http.MethodGet, node.URL, "/router/v1/node/load-errors?"+forwarded.Encode(), nil, &response)
	return response, err
}

func (service *Service) localLoadErrorList(ctx context.Context, filter loaderrors.ListFilter) (loadErrorListResponse, error) {
	enabled := service.loadErrorStore != nil
	response := loadErrorListResponse{Enabled: enabled, Nodes: []loadCaptureNode{{NodeID: service.nodeID, Enabled: enabled}}, Records: []loaderrors.Record{}}
	if !enabled {
		return response, nil
	}
	result, err := service.loadErrorStore.List(ctx, filter)
	if err != nil {
		return response, err
	}
	response.Records = result.Records
	return response, nil
}

func parseLoadErrorQuery(values url.Values) (loaderrors.ListFilter, error) {
	filter := loaderrors.ListFilter{
		NodeID:   strings.TrimSpace(values.Get("node_id")),
		Phase:    loaderrors.Phase(strings.TrimSpace(values.Get("phase"))),
		Severity: loaderrors.Severity(strings.TrimSpace(values.Get("severity"))),
		Limit:    200,
	}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 500 {
			return filter, fmt.Errorf("limit must be between 1 and 500")
		}
		filter.Limit = limit
	}
	if filter.Severity != "" && filter.Severity != loaderrors.SeverityError && filter.Severity != loaderrors.SeverityWarning {
		return filter, fmt.Errorf("severity must be error or warning")
	}
	return filter, nil
}
