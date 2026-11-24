package proxy

import (
	"encoding/json"
	"net/http"
	"strings"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
)

type securityNodeIssueRequest struct {
	AuthorityID string   `json:"authority_id"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Hosts       []string `json:"hosts"`
	Activate    bool     `json:"activate"`
}

func (relay *securityNodeRelay) issue(w http.ResponseWriter, r *http.Request, node securityNodeClient, requestJSON []byte) {
	var request securityNodeIssueRequest
	if !decodeSecurityRelayBody(w, requestJSON, &request) {
		return
	}
	kind, ok := issuableKind(w, request)
	if !ok {
		return
	}
	var pending credentialstore.PendingKey
	create := securityCertificateRequestCreate{Name: request.Name, Kind: string(kind), Hosts: request.Hosts}
	if err := node.call(r.Context(), http.MethodPost, "csr", create, &pending); err != nil {
		writeSecurityRelayError(w, err)
		return
	}
	if err := verifyNodeCertificateRequest(pending.RequestPEM, request.Name, request.Hosts); err != nil {
		node.discardPendingKey(r, pending.ID)
		openai.WriteError(w, http.StatusBadGateway, "node_error", err.Error())
		return
	}
	chainPEM, err := relay.store().SignCertificateRequest(r.Context(), request.AuthorityID, kind, pending.RequestPEM)
	if err != nil {
		node.discardPendingKey(r, pending.ID)
		writeCredentialError(w, err)
		return
	}
	installJSON, err := json.Marshal(securitySignedCertificateInstall{CertificatePEM: chainPEM, Activate: request.Activate})
	if err != nil {
		node.discardPendingKey(r, pending.ID)
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
		return
	}
	installed, err := node.exchange(r.Context(), http.MethodPost, "install", installJSON)
	if err != nil {
		node.discardPendingKey(r, pending.ID)
		writeSecurityRelayError(w, err)
		return
	}
	writeRelayedResponse(w, installed)
}

func issuableKind(w http.ResponseWriter, request securityNodeIssueRequest) (credentialstore.Kind, bool) {
	if strings.TrimSpace(request.AuthorityID) == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "authority_id is required")
		return "", false
	}
	kind, err := credentialstore.ParseKind(request.Kind)
	if err != nil {
		writeCredentialError(w, err)
		return "", false
	}
	if !kind.Activatable() {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "kind must be identity or api_certificate")
		return "", false
	}
	return kind, true
}
