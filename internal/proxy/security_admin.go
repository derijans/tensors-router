package proxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
	"tensors-router/internal/proxy/routing"
)

const (
	securityTokensPath       = "/router/v1/site/security/tokens"
	securityCertificatesPath = "/router/v1/site/security/certificates"
	securityAuthoritiesPath  = "/router/v1/site/security/authorities"
	securityStatusPath       = "/router/v1/site/security/status"
)

type SecurityConfig struct {
	Credentials          *credentialstore.Live
	DefaultIdentityHosts []string
	ListenerTLS          bool
}

type securityAdmin struct {
	credentials          *credentialstore.Live
	defaultIdentityHosts []string
	listenerTLS          bool
}

func newSecurityAdmin(config SecurityConfig) *securityAdmin {
	return &securityAdmin{
		credentials:          config.Credentials,
		defaultIdentityHosts: append([]string{}, config.DefaultIdentityHosts...),
		listenerTLS:          config.ListenerTLS,
	}
}

func (admin *securityAdmin) routes() []routing.Route {
	return []routing.Route{
		routing.Exact(http.MethodGet, securityStatusPath, admin.guarded(admin.handleStatus)),
		routing.Exact(routing.AnyMethod, securityTokensPath, admin.guarded(admin.handleTokens)),
		routing.Prefix(routing.AnyMethod, securityTokensPath+"/", admin.guarded(admin.handleToken)),
		routing.Exact(routing.AnyMethod, securityCertificatesPath, admin.guarded(admin.handleCertificates)),
		routing.Prefix(routing.AnyMethod, securityCertificatesPath+"/", admin.guarded(admin.handleCertificate)),
		routing.Exact(http.MethodPost, securityAuthoritiesPath, admin.guarded(admin.handleCreateAuthority)),
		routing.Prefix(http.MethodPost, securityAuthoritiesPath+"/", admin.guarded(admin.handleIssueIdentity)),
	}
}

func (admin *securityAdmin) guarded(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if admin.credentials == nil {
			openai.WriteError(w, http.StatusNotFound, "not_found", "credential store is not available on this router")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		handler(w, r)
	}
}

func (admin *securityAdmin) store() *credentialstore.Store {
	return admin.credentials.Store()
}

type securityStatusResponse struct {
	ListenerTLS     bool `json:"listener_tls"`
	IdentityActive  bool `json:"identity_active"`
	RestartRequired bool `json:"restart_required"`
}

func (admin *securityAdmin) handleStatus(w http.ResponseWriter, _ *http.Request) {
	identityActive := admin.credentials.Current().TLSEnabled()
	openai.WriteJSON(w, http.StatusOK, securityStatusResponse{
		ListenerTLS:     admin.listenerTLS,
		IdentityActive:  identityActive,
		RestartRequired: identityActive != admin.listenerTLS,
	})
}

func (admin *securityAdmin) refreshAfterWrite(r *http.Request) error {
	return admin.credentials.Refresh(r.Context())
}

func resourceIDAndAction(path string, prefix string) (string, string) {
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	id, action, _ := strings.Cut(rest, "/")
	return id, action
}

func decodeSecurityRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func writeCredentialError(w http.ResponseWriter, err error) {
	var input credentialstore.InputError
	switch {
	case errors.As(err, &input):
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", input.Error())
	case errors.Is(err, credentialstore.ErrNotFound):
		openai.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, credentialstore.ErrConflict):
		openai.WriteError(w, http.StatusConflict, "conflict", "a credential with the same name or value already exists")
	default:
		openai.WriteError(w, http.StatusInternalServerError, "server_error", err.Error())
	}
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	openai.WriteError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
}
