package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"tensors-router/internal/cluster"
	"tensors-router/internal/openai"
)

const securityRelayBodyLimit = 1 << 20

type securityNodeClient struct {
	deps    securityRelayDeps
	nodeURL string
}

func (node securityNodeClient) exchange(ctx context.Context, method string, suffix string, requestJSON []byte) (cluster.RawResponse, error) {
	return node.deps.exchangeWithNode(ctx, method, node.nodeURL, securityNodePath+"/"+suffix, requestJSON)
}

func (node securityNodeClient) call(ctx context.Context, method string, suffix string, request any, response any) error {
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return err
	}
	raw, err := node.exchange(ctx, method, suffix, requestJSON)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw.Body, response)
}

func (node securityNodeClient) relayJSON(w http.ResponseWriter, r *http.Request, suffix string, request any) {
	requestJSON, err := json.Marshal(request)
	if err != nil {
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	node.relay(w, r, http.MethodPost, suffix, requestJSON)
}

func (node securityNodeClient) relay(w http.ResponseWriter, r *http.Request, method string, suffix string, requestJSON []byte) {
	response, err := node.exchange(r.Context(), method, suffix, requestJSON)
	if err != nil {
		writeSecurityRelayError(w, err)
		return
	}
	writeRelayedResponse(w, response)
}

func (node securityNodeClient) discardPendingKey(r *http.Request, id string) {
	if !relayableSegment(id) {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), cluster.DefaultControlTimeout)
	defer cancel()
	_, _ = node.exchange(ctx, http.MethodDelete, "csr/"+id, nil)
}

func readSecurityRelayBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if r.Body == nil || r.Method == http.MethodGet || r.Method == http.MethodDelete {
		return nil, true
	}
	content, err := io.ReadAll(io.LimitReader(r.Body, securityRelayBodyLimit+1))
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return nil, false
	}
	if len(content) > securityRelayBodyLimit {
		openai.WriteError(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "request body is too large")
		return nil, false
	}
	if len(content) == 0 {
		return nil, true
	}
	return content, true
}

func decodeSecurityRelayBody(w http.ResponseWriter, requestJSON []byte, target any) bool {
	if len(requestJSON) == 0 {
		return true
	}
	if err := strictJSONUnmarshal(requestJSON, target); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeRelayedResponse(w http.ResponseWriter, response cluster.RawResponse) {
	w.Header().Set("Content-Type", relayedContentType(response.ContentType))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(response.StatusCode)
	if response.StatusCode != http.StatusNoContent {
		_, _ = w.Write(response.Body)
	}
}

func relayedContentType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err == nil && (mediaType == "application/json" || mediaType == "application/x-pem-file") {
		return contentType
	}
	return "application/octet-stream"
}

func writeSecurityRelayError(w http.ResponseWriter, err error) {
	var remoteError *cluster.RemoteError
	if !errors.As(err, &remoteError) {
		openai.WriteError(w, http.StatusBadGateway, "node_unreachable", "node request failed: "+err.Error())
		return
	}
	status := remoteError.StatusCode
	if status < 400 || status > 599 {
		status = http.StatusBadGateway
	}
	errorType := remoteError.Type
	if errorType == "" {
		errorType = "node_error"
	}
	message := remoteError.Message
	if message == "" {
		message = err.Error()
	}
	openai.WriteError(w, status, errorType, message)
}
