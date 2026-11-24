package proxy

import (
	"net/http"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
)

type securityTokensResponse struct {
	Tokens []credentialstore.Token `json:"tokens"`
}

type securityTokenCreateRequest struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	Value string `json:"value"`
}

type securityTokenCreatedResponse struct {
	Token  credentialstore.Token `json:"token"`
	Secret string                `json:"secret"`
}

type securityTokenSecretResponse struct {
	Secret string `json:"secret"`
}

func (admin *securityAdmin) handleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		admin.writeTokens(w, r)
	case http.MethodPost:
		admin.createToken(w, r)
	default:
		writeMethodNotAllowed(w)
	}
}

func (admin *securityAdmin) writeTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := admin.store().Tokens(r.Context())
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, securityTokensResponse{Tokens: tokens})
}

func (admin *securityAdmin) createToken(w http.ResponseWriter, r *http.Request) {
	var request securityTokenCreateRequest
	if !decodeSecurityRequest(w, r, &request) {
		return
	}
	role, err := credentialstore.ParseRole(request.Role)
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	token, secret, err := admin.store().CreateToken(r.Context(), credentialstore.NewToken{Name: request.Name, Role: role, Supplied: request.Value})
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	if err := admin.refreshAfterWrite(r); err != nil {
		writeCredentialError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusCreated, securityTokenCreatedResponse{Token: token, Secret: secret})
}

func (admin *securityAdmin) handleToken(w http.ResponseWriter, r *http.Request) {
	id, action := resourceIDAndAction(r.URL.Path, securityTokensPath)
	switch {
	case action == "" && r.Method == http.MethodDelete:
		admin.deleteToken(w, r, id)
	case action == "secret" && r.Method == http.MethodGet:
		admin.revealClusterSecret(w, r, id)
	default:
		openai.WriteEndpointNotFound(w)
	}
}

func (admin *securityAdmin) deleteToken(w http.ResponseWriter, r *http.Request, id string) {
	if err := admin.store().DeleteToken(r.Context(), id); err != nil {
		writeCredentialError(w, err)
		return
	}
	if err := admin.refreshAfterWrite(r); err != nil {
		writeCredentialError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (admin *securityAdmin) revealClusterSecret(w http.ResponseWriter, r *http.Request, id string) {
	secret, err := admin.store().ClusterSecret(r.Context(), id)
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, securityTokenSecretResponse{Secret: secret})
}
