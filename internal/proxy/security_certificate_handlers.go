package proxy

import (
	"net/http"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
)

type securityCertificatesResponse struct {
	Certificates []credentialstore.Certificate `json:"certificates"`
	DefaultHosts []string                      `json:"default_hosts"`
}

type securityCertificateImportRequest struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	CertificatePEM string `json:"certificate_pem"`
	PrivateKeyPEM  string `json:"private_key_pem"`
	Activate       bool   `json:"activate"`
}

type securityIdentityGenerateRequest struct {
	Name     string   `json:"name"`
	Hosts    []string `json:"hosts"`
	Activate bool     `json:"activate"`
}

func (admin *securityAdmin) handleCertificates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		admin.writeCertificates(w, r)
	case http.MethodPost:
		admin.importCertificate(w, r)
	default:
		writeMethodNotAllowed(w)
	}
}

func (admin *securityAdmin) writeCertificates(w http.ResponseWriter, r *http.Request) {
	certificates, err := admin.store().Certificates(r.Context())
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, securityCertificatesResponse{Certificates: certificates, DefaultHosts: admin.defaultIdentityHosts})
}

func (admin *securityAdmin) importCertificate(w http.ResponseWriter, r *http.Request) {
	var request securityCertificateImportRequest
	if !decodeSecurityRequest(w, r, &request) {
		return
	}
	kind, err := credentialstore.ParseKind(request.Kind)
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	certificate, err := admin.store().ImportCertificate(r.Context(), credentialstore.CertificateImport{
		Name:           request.Name,
		Kind:           kind,
		CertificatePEM: request.CertificatePEM,
		PrivateKeyPEM:  request.PrivateKeyPEM,
	})
	admin.finishCertificateWrite(w, r, certificate, request.Activate, err)
}

func (admin *securityAdmin) generateIdentity(w http.ResponseWriter, r *http.Request) {
	var request securityIdentityGenerateRequest
	if !decodeSecurityRequest(w, r, &request) {
		return
	}
	hosts := request.Hosts
	if len(hosts) == 0 {
		hosts = admin.defaultIdentityHosts
	}
	certificate, err := admin.store().GenerateIdentity(r.Context(), request.Name, hosts)
	admin.finishCertificateWrite(w, r, certificate, request.Activate, err)
}

func (admin *securityAdmin) finishCertificateWrite(w http.ResponseWriter, r *http.Request, certificate credentialstore.Certificate, activate bool, err error) {
	if err == nil && activate {
		err = admin.store().Activate(r.Context(), certificate.ID)
		certificate.Active = err == nil
	}
	if err == nil {
		err = admin.refreshAfterWrite(r)
	}
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusCreated, certificate)
}

func (admin *securityAdmin) handleCertificate(w http.ResponseWriter, r *http.Request) {
	id, action := resourceIDAndAction(r.URL.Path, securityCertificatesPath)
	switch {
	case id == "generate" && action == "" && r.Method == http.MethodPost:
		admin.generateIdentity(w, r)
	case action == "pem" && r.Method == http.MethodGet:
		admin.writePublicCertificate(w, r, id)
	case action == "activate" && r.Method == http.MethodPost:
		admin.activateCertificate(w, r, id)
	case action == "" && r.Method == http.MethodDelete:
		admin.deleteCertificate(w, r, id)
	default:
		openai.WriteEndpointNotFound(w)
	}
}

func (admin *securityAdmin) activateCertificate(w http.ResponseWriter, r *http.Request, id string) {
	err := admin.store().Activate(r.Context(), id)
	if err == nil {
		err = admin.refreshAfterWrite(r)
	}
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (admin *securityAdmin) deleteCertificate(w http.ResponseWriter, r *http.Request, id string) {
	err := admin.store().DeleteCertificate(r.Context(), id, r.URL.Query().Get("force") == "true")
	if err == nil {
		err = admin.refreshAfterWrite(r)
	}
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (admin *securityAdmin) writePublicCertificate(w http.ResponseWriter, r *http.Request, id string) {
	certificatePEM, err := admin.store().PublicCertificatePEM(r.Context(), id)
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-pem-file")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(certificatePEM))
}
