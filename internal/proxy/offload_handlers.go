package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tensors-router/internal/openai"
)

const (
	offloadMarkerHeader = "X-Tensors-Offload"
	offloadGroupHeader  = "X-Tensors-Offload-Group"
	offloadOwnerHeader  = "X-Tensors-Offload-Owner"
	offloadPathHeader   = "X-Tensors-Offload-Path"

	offloadReturnedCode = "offload_returned"
)

type offloadContextKey struct{}

// markBorrowedRequest records that a request arrived as borrowed work. The flag
// lives on the context rather than in a header so nothing downstream can be fooled
// by a client that sets the header itself: only handleNodeInference, which is
// already cluster-token gated, ever puts it there.
func markBorrowedRequest(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), offloadContextKey{}, true))
}

func requestIsBorrowed(r *http.Request) bool {
	borrowed, _ := r.Context().Value(offloadContextKey{}).(bool)
	return borrowed
}

// handleNodeOffloadGrant receives a slot lease from the master. The owner keeps it
// until it expires; nothing revokes it.
func (service *Service) handleNodeOffloadGrant(w http.ResponseWriter, r *http.Request) {
	var lease offloadLease
	if err := json.NewDecoder(r.Body).Decode(&lease); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if strings.TrimSpace(lease.GroupID) == "" || strings.TrimSpace(lease.HelperNodeID) == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "group_id and helper_node_id are required")
		return
	}
	service.storeOffloadLease(lease)
	openai.WriteJSON(w, http.StatusOK, lease)
}

// handleNodeOffloadRequest is the master relaying one borrowed request from an
// owner to the helper its lease names. The master brokers the placement but never
// takes custody: the response goes straight back to the owner, which is still
// holding the client.
func (service *Service) handleNodeOffloadRequest(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.Header.Get(offloadGroupHeader))
	ownerNodeID := strings.TrimSpace(r.Header.Get(offloadOwnerHeader))
	path := strings.TrimSpace(r.Header.Get(offloadPathHeader))
	if groupID == "" || ownerNodeID == "" || !isLocalInferencePath(path) {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "offload group, owner, and path are required")
		return
	}
	lease, ok := service.leaseBook.Lease(groupID, ownerNodeID, time.Now())
	if !ok {
		openai.WriteError(w, http.StatusConflict, offloadReturnedCode, "no live offload lease for this owner")
		return
	}
	nodeURL := service.registry.NodeURLsByID()[lease.HelperNodeID]
	if nodeURL == "" {
		openai.WriteError(w, http.StatusConflict, offloadReturnedCode, "helper node is not reachable")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	response, err := service.forwardBorrowedRequest(r.Context(), nodeURL, path, r, body)
	if err != nil {
		openai.WriteError(w, http.StatusConflict, offloadReturnedCode, err.Error())
		return
	}
	defer func() { _ = response.Body.Close() }()
	copyBackendHeaders(w.Header(), response.Header)
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func (service *Service) forwardBorrowedRequest(ctx context.Context, nodeURL string, path string, original *http.Request, body []byte) (*http.Response, error) {
	base, err := service.clusterClient.AuthorizedBaseURL(nodeURL)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	target.Path = joinPath(target.Path, "/router/v1/node/inference"+path)
	target.RawQuery = original.URL.RawQuery

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyClusterRequestHeaders(request.Header, original.Header)
	request.Header.Set("Authorization", "Bearer "+service.clusterToken)
	request.Header.Set(offloadMarkerHeader, "1")
	request.Host = target.Host
	return service.client.Do(request)
}

// sendOffloadedRequest is the owner handing one request to the master for
// placement. It returns errOffloadReturned when the helper could not take it, in
// which case the caller re-queues locally and answers its client itself.
func (service *Service) sendOffloadedRequest(ctx context.Context, groupID string, r *http.Request, body []byte) (*http.Response, error) {
	masterURL, err := service.clusterClient.AuthorizedBaseURL(service.masterURL)
	if err != nil {
		return nil, err
	}
	target, err := url.Parse(masterURL)
	if err != nil {
		return nil, err
	}
	target.Path = joinPath(target.Path, "/router/v1/node/offload/request")
	target.RawQuery = r.URL.RawQuery

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	copyClusterRequestHeaders(request.Header, r.Header)
	request.Header.Set("Authorization", "Bearer "+service.clusterToken)
	request.Header.Set(offloadGroupHeader, groupID)
	request.Header.Set(offloadOwnerHeader, service.nodeID)
	request.Header.Set(offloadPathHeader, r.URL.Path)
	request.Host = target.Host

	response, err := service.client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode == http.StatusConflict {
		_ = response.Body.Close()
		return nil, errOffloadReturned
	}
	return response, nil
}
