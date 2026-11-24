package proxy

import (
	"net/http"

	"tensors-router/internal/openai"
)

type securityAuthorityCreateRequest struct {
	Name string `json:"name"`
}

type securityIssueRequest struct {
	Name           string   `json:"name"`
	Hosts          []string `json:"hosts"`
	InstallLocally bool     `json:"install_locally"`
	Activate       bool     `json:"activate"`
}

type securityIssuedResponse struct {
	CertificatePEM string `json:"certificate_pem"`
	PrivateKeyPEM  string `json:"private_key_pem"`
}

func (admin *securityAdmin) handleCreateAuthority(w http.ResponseWriter, r *http.Request) {
	var request securityAuthorityCreateRequest
	if !decodeSecurityRequest(w, r, &request) {
		return
	}
	certificate, err := admin.store().CreateAuthority(r.Context(), request.Name)
	admin.finishCertificateWrite(w, r, certificate, false, err)
}

func (admin *securityAdmin) handleIssueIdentity(w http.ResponseWriter, r *http.Request) {
	authorityID, action := resourceIDAndAction(r.URL.Path, securityAuthoritiesPath)
	if action != "issue" {
		openai.WriteEndpointNotFound(w)
		return
	}
	var request securityIssueRequest
	if !decodeSecurityRequest(w, r, &request) {
		return
	}
	hosts := request.Hosts
	if len(hosts) == 0 && request.InstallLocally {
		hosts = admin.defaultIdentityHosts
	}
	issued, err := admin.store().IssueIdentity(r.Context(), authorityID, request.Name, hosts)
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	if !request.InstallLocally {
		openai.WriteJSON(w, http.StatusCreated, securityIssuedResponse{CertificatePEM: issued.CertificatePEM, PrivateKeyPEM: issued.PrivateKeyPEM})
		return
	}
	certificate, err := admin.store().ImportCertificate(r.Context(), issued.AsIdentityImport(request.Name))
	admin.finishCertificateWrite(w, r, certificate, request.Activate, err)
}
