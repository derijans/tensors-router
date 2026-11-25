package proxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
)

const (
	securitySitePath         = "/router/v1/site/security"
	securityNodePath         = "/router/v1/node/security"
	securityTokensPath       = securitySitePath + "/tokens"
	securityCertificatesPath = securitySitePath + "/certificates"
	securityAuthoritiesPath  = securitySitePath + "/authorities"
	securityStatusPath       = securitySitePath + "/status"
)

type SecurityConfig struct {
	Credentials          *credentialstore.Live
	DefaultIdentityHosts []string
	Listeners            []SecurityListener
	Profile              SecurityProfileControl
	Cluster              SecurityClusterSettings
}

type SecurityListener struct {
	Name     string
	Bind     string
	TLS      bool
	WantsTLS func() bool
}

type securityAdmin struct {
	nodeID               string
	credentials          *credentialstore.Live
	defaultIdentityHosts []string
	listeners            []SecurityListener
	profile              *securityProfile
	cluster              SecurityClusterSettings
}

func newSecurityAdmin(config SecurityConfig, nodeID string) *securityAdmin {
	return &securityAdmin{
		nodeID:               nodeID,
		credentials:          config.Credentials,
		defaultIdentityHosts: append([]string{}, config.DefaultIdentityHosts...),
		listeners:            append([]SecurityListener{}, config.Listeners...),
		profile:              newSecurityProfile(config.Profile),
		cluster:              config.Cluster.clone(),
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
	IdentityActive       bool                     `json:"identity_active"`
	APICertificateActive bool                     `json:"api_certificate_active"`
	Listeners            []securityListenerStatus `json:"listeners"`
	securityProfileStatus
}

type securityListenerStatus struct {
	Name            string `json:"name"`
	Bind            string `json:"bind"`
	TLS             bool   `json:"tls"`
	RestartRequired bool   `json:"restart_required"`
}

func (admin *securityAdmin) handleStatus(w http.ResponseWriter, _ *http.Request) {
	openai.WriteJSON(w, http.StatusOK, admin.status())
}

func (admin *securityAdmin) status() securityStatusResponse {
	snapshot := admin.credentials.Current()
	listeners := make([]securityListenerStatus, 0, len(admin.listeners))
	for _, listener := range admin.listeners {
		listeners = append(listeners, securityListenerStatus{
			Name:            listener.Name,
			Bind:            listener.Bind,
			TLS:             listener.TLS,
			RestartRequired: listener.WantsTLS != nil && listener.WantsTLS() != listener.TLS,
		})
	}
	return securityStatusResponse{
		IdentityActive:        snapshot.TLSEnabled(),
		APICertificateActive:  snapshot.APICertificate != nil,
		Listeners:             listeners,
		securityProfileStatus: admin.profile.status(),
	}
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
