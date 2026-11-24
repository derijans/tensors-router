package proxy

import (
	"net/http"
	"strings"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
)

type securityCertificateRequestCreate struct {
	Name  string   `json:"name"`
	Kind  string   `json:"kind"`
	Hosts []string `json:"hosts"`
}

type securitySignedCertificateInstall struct {
	CertificatePEM string `json:"certificate_pem"`
	Activate       bool   `json:"activate"`
}

func (admin *securityAdmin) createPendingKey(w http.ResponseWriter, r *http.Request) {
	var request securityCertificateRequestCreate
	if !decodeSecurityRequest(w, r, &request) {
		return
	}
	hosts := request.Hosts
	if len(hosts) == 0 {
		hosts = admin.defaultIdentityHosts
	}
	pending, err := admin.store().CreatePendingKey(r.Context(), credentialstore.PendingKeyRequest{
		Name:  request.Name,
		Kind:  credentialstore.Kind(request.Kind),
		Hosts: hosts,
	})
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusCreated, pending)
}

func (admin *securityAdmin) handlePendingKeyDeletion(requestsPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, action := resourceIDAndAction(r.URL.Path, requestsPath)
		if action != "" {
			openai.WriteEndpointNotFound(w)
			return
		}
		if err := admin.store().DeletePendingKey(r.Context(), id); err != nil {
			writeCredentialError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (admin *securityAdmin) installSignedCertificate(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request securitySignedCertificateInstall
		if !decodeSecurityRequest(w, r, &request) || rejectPrivateKeyMaterial(w, request.CertificatePEM) {
			return
		}
		certificate, err := admin.store().InstallSignedCertificate(r.Context(), credentialstore.SignedCertificateInstall{
			CertificatePEM: request.CertificatePEM,
			Activate:       request.Activate,
			Source:         source,
		})
		if err == nil {
			err = admin.refreshAfterWrite(r)
		}
		if err != nil {
			writeCredentialError(w, err)
			return
		}
		openai.WriteJSON(w, http.StatusCreated, certificate)
	}
}

func rejectPrivateKeyMaterial(w http.ResponseWriter, certificatePEM string) bool {
	if !strings.Contains(certificatePEM, "PRIVATE KEY") {
		return false
	}
	openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "certificate_pem must not contain a private key")
	return true
}
