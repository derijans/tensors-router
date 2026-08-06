package proxy

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"tensors-router/internal/loadcapture"
	"tensors-router/internal/openai"
)

type loadCaptureNode struct {
	NodeID  string `json:"node_id"`
	Enabled bool   `json:"enabled"`
}

type loadCaptureNodeError struct {
	NodeID string `json:"node_id"`
	Error  string `json:"error"`
}

type loadCaptureListResponse struct {
	Enabled    bool                   `json:"enabled"`
	Nodes      []loadCaptureNode      `json:"nodes"`
	Attempts   []loadcapture.Attempt  `json:"attempts"`
	NextCursor string                 `json:"next_cursor,omitempty"`
	NodeErrors []loadCaptureNodeError `json:"node_errors,omitempty"`
}

type loadCaptureDetailResponse struct {
	Attempt        loadcapture.Attempt `json:"attempt"`
	SnapshotSHA256 string              `json:"snapshot_sha256"`
	KCPPS          json.RawMessage     `json:"kcpps"`
	Assets         []loadcapture.Asset `json:"assets"`
}

type loadCaptureCursor struct {
	StartedAt int64  `json:"started_at"`
	AttemptID string `json:"attempt_id"`
}

func (service *Service) handleSiteLoadCaptures(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	query, cursor, err := parseLoadCaptureQuery(r.URL.Query())
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	selected, err := service.selectedLoadCaptureNodes(r.URL.Query()["node_id"])
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response := loadCaptureListResponse{Attempts: []loadcapture.Attempt{}, Nodes: []loadCaptureNode{}}
	for _, node := range selected {
		result, requestErr := service.loadCapturesFromNode(r.Context(), node.NodeID, node.URL, query, cursor, r.URL.Query())
		if requestErr != nil {
			response.NodeErrors = append(response.NodeErrors, loadCaptureNodeError{NodeID: node.NodeID, Error: requestErr.Error()})
			continue
		}
		response.Nodes = append(response.Nodes, result.Nodes...)
		response.Attempts = append(response.Attempts, result.Attempts...)
		response.Enabled = response.Enabled || result.Enabled
	}
	sort.Slice(response.Nodes, func(left int, right int) bool { return response.Nodes[left].NodeID < response.Nodes[right].NodeID })
	sort.Slice(response.Attempts, func(left int, right int) bool {
		if response.Attempts[left].StartedAt.Equal(response.Attempts[right].StartedAt) {
			return response.Attempts[left].ID > response.Attempts[right].ID
		}
		return response.Attempts[left].StartedAt.After(response.Attempts[right].StartedAt)
	})
	if len(response.Attempts) > query.Limit {
		response.Attempts = response.Attempts[:query.Limit]
	}
	if len(response.Attempts) == query.Limit {
		last := response.Attempts[len(response.Attempts)-1]
		response.NextCursor = encodeLoadCaptureCursor(loadCaptureCursor{StartedAt: last.StartedAt.UnixMilli(), AttemptID: last.ID})
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

type loadCaptureNodeTarget struct {
	NodeID string
	URL    string
}

func (service *Service) selectedLoadCaptureNodes(requested []string) ([]loadCaptureNodeTarget, error) {
	available := map[string]string{service.nodeID: ""}
	if service.registry != nil {
		for nodeID, nodeURL := range service.registry.NodeURLsByID() {
			if nodeID != service.nodeID {
				available[nodeID] = nodeURL
			}
		}
	}
	selectedIDs := requested
	if len(selectedIDs) == 0 {
		selectedIDs = make([]string, 0, len(available))
		for nodeID := range available {
			selectedIDs = append(selectedIDs, nodeID)
		}
	}
	unique := map[string]struct{}{}
	result := []loadCaptureNodeTarget{}
	for _, nodeID := range selectedIDs {
		nodeID = strings.TrimSpace(nodeID)
		nodeURL, ok := available[nodeID]
		if !ok || nodeID == "" {
			return nil, fmt.Errorf("unknown node_id %q", nodeID)
		}
		if _, exists := unique[nodeID]; exists {
			continue
		}
		unique[nodeID] = struct{}{}
		result = append(result, loadCaptureNodeTarget{NodeID: nodeID, URL: nodeURL})
	}
	sort.Slice(result, func(left int, right int) bool { return result[left].NodeID < result[right].NodeID })
	return result, nil
}

func (service *Service) loadCapturesFromNode(ctx context.Context, nodeID string, nodeURL string, query loadcapture.ListQuery, cursor loadCaptureCursor, values url.Values) (loadCaptureListResponse, error) {
	if nodeID == service.nodeID {
		return service.localLoadCaptureList(ctx, query)
	}
	forwarded := cloneQueryWithout(values, "node_id")
	if cursor.AttemptID != "" {
		forwarded.Set("cursor", encodeLoadCaptureCursor(cursor))
	}
	var response loadCaptureListResponse
	err := service.clusterClient.JSON(ctx, http.MethodGet, nodeURL, "/router/v1/node/load-captures?"+forwarded.Encode(), nil, &response)
	return response, err
}

func (service *Service) handleNodeLoadCaptures(w http.ResponseWriter, r *http.Request) {
	query, _, err := parseLoadCaptureQuery(r.URL.Query())
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response, err := service.localLoadCaptureList(r.Context(), query)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, response)
}

func (service *Service) localLoadCaptureList(ctx context.Context, query loadcapture.ListQuery) (loadCaptureListResponse, error) {
	enabled := service.loadCaptureStore != nil
	response := loadCaptureListResponse{Enabled: enabled, Nodes: []loadCaptureNode{{NodeID: service.nodeID, Enabled: enabled}}, Attempts: []loadcapture.Attempt{}}
	if !enabled {
		return response, nil
	}
	attempts, err := service.loadCaptureStore.ListFiltered(ctx, query)
	if err != nil {
		return response, err
	}
	response.Attempts = attempts
	return response, nil
}

func (service *Service) handleSiteLoadCaptureRecord(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
	if nodeID == "" {
		nodeID = service.nodeID
	}
	nodes, err := service.selectedLoadCaptureNodes([]string{nodeID})
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	attemptID, output, ok := loadCaptureRecordPath(r.URL.Path, "/router/v1/site/load-captures/")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "capture not found")
		return
	}
	if nodes[0].NodeID == service.nodeID {
		service.writeLocalLoadCaptureRecord(w, r, attemptID, output)
		return
	}
	targetPath := "/router/v1/node/load-captures/" + url.PathEscape(attemptID)
	if output {
		targetPath += "/output"
	}
	forwarded := cloneQueryWithout(r.URL.Query(), "node_id")
	if encoded := forwarded.Encode(); encoded != "" {
		targetPath += "?" + encoded
	}
	response, err := service.clusterClient.Stream(r.Context(), http.MethodGet, nodes[0].URL, targetPath)
	if err != nil {
		openai.WriteError(w, http.StatusBadGateway, "node_error", err.Error())
		return
	}
	defer response.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (service *Service) handleNodeLoadCaptureRecord(w http.ResponseWriter, r *http.Request) {
	attemptID, output, ok := loadCaptureRecordPath(r.URL.Path, "/router/v1/node/load-captures/")
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "capture not found")
		return
	}
	service.writeLocalLoadCaptureRecord(w, r, attemptID, output)
}

func (service *Service) writeLocalLoadCaptureRecord(w http.ResponseWriter, r *http.Request, attemptID string, output bool) {
	if service.loadCaptureStore == nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "load capture is disabled")
		return
	}
	if output {
		after, err := nonNegativeQueryInt(r.URL.Query().Get("after_sequence"))
		if err != nil {
			openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		page, err := service.loadCaptureStore.Output(r.Context(), attemptID, after, 16)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				openai.WriteError(w, http.StatusNotFound, "not_found", "capture not found")
				return
			}
			openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
			return
		}
		openai.WriteJSON(w, http.StatusOK, page)
		return
	}
	detail, err := service.loadCaptureStore.Detail(r.Context(), attemptID)
	if err != nil {
		openai.WriteError(w, http.StatusNotFound, "not_found", "capture not found")
		return
	}
	openai.WriteJSON(w, http.StatusOK, loadCaptureDetailResponse{Attempt: detail.Attempt, SnapshotSHA256: detail.Snapshot.SHA256, KCPPS: json.RawMessage(detail.Snapshot.JSON), Assets: detail.Assets})
}

func parseLoadCaptureQuery(values url.Values) (loadcapture.ListQuery, loadCaptureCursor, error) {
	query := loadcapture.ListQuery{Limit: 100, Status: loadcapture.Status(strings.TrimSpace(values.Get("status"))), Kind: loadcapture.Kind(strings.TrimSpace(values.Get("kind"))), BackendMode: strings.TrimSpace(values.Get("backend"))}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			return query, loadCaptureCursor{}, fmt.Errorf("limit must be between 1 and 200")
		}
		query.Limit = limit
	}
	if query.Status != "" && query.Status != loadcapture.StatusLoading && query.Status != loadcapture.StatusSucceeded && query.Status != loadcapture.StatusFailed && query.Status != loadcapture.StatusInterrupted && query.Status != loadcapture.StatusReused {
		return query, loadCaptureCursor{}, fmt.Errorf("invalid load capture status %q", query.Status)
	}
	if query.Kind != "" && query.Kind != loadcapture.KindPhysical && query.Kind != loadcapture.KindReuse {
		return query, loadCaptureCursor{}, fmt.Errorf("invalid load capture kind %q", query.Kind)
	}
	for _, value := range []struct {
		raw    string
		target *int64
	}{{raw: values.Get("from"), target: &query.FromMS}, {raw: values.Get("to"), target: &query.ToMS}} {
		if strings.TrimSpace(value.raw) == "" {
			continue
		}
		parsed, err := strconv.ParseInt(value.raw, 10, 64)
		if err != nil || parsed < 0 {
			return query, loadCaptureCursor{}, fmt.Errorf("time filters must be non-negative Unix milliseconds")
		}
		*value.target = parsed
	}
	cursor, err := decodeLoadCaptureCursor(values.Get("cursor"))
	if err != nil {
		return query, cursor, err
	}
	query.BeforeStartedMS = cursor.StartedAt
	query.BeforeID = cursor.AttemptID
	return query, cursor, nil
}

func encodeLoadCaptureCursor(cursor loadCaptureCursor) string {
	content, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(content)
}

func decodeLoadCaptureCursor(value string) (loadCaptureCursor, error) {
	if strings.TrimSpace(value) == "" {
		return loadCaptureCursor{}, nil
	}
	content, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return loadCaptureCursor{}, fmt.Errorf("invalid cursor")
	}
	var cursor loadCaptureCursor
	if json.Unmarshal(content, &cursor) != nil || cursor.StartedAt <= 0 || strings.TrimSpace(cursor.AttemptID) == "" {
		return loadCaptureCursor{}, fmt.Errorf("invalid cursor")
	}
	return cursor, nil
}

func loadCaptureRecordPath(path string, prefix string) (string, bool, bool) {
	remainder := strings.TrimPrefix(path, prefix)
	if remainder == path || remainder == "" {
		return "", false, false
	}
	output := strings.HasSuffix(remainder, "/output")
	if output {
		remainder = strings.TrimSuffix(remainder, "/output")
	}
	attemptID, err := url.PathUnescape(remainder)
	return attemptID, output, err == nil && attemptID != "" && !strings.Contains(attemptID, "/")
}

func cloneQueryWithout(values url.Values, excluded string) url.Values {
	result := url.Values{}
	for key, entries := range values {
		if key == excluded {
			continue
		}
		result[key] = append([]string{}, entries...)
	}
	return result
}

func nonNegativeQueryInt(value string) (int64, error) {
	if strings.TrimSpace(value) == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("after_sequence must be non-negative")
	}
	return parsed, nil
}
