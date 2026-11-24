package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
)

type securityTrustImport struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	CertificatePEM string `json:"certificate_pem"`
}

type securityClusterTokenImport struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type securityClusterTokenResult struct {
	Token   credentialstore.Token `json:"token"`
	Created bool                  `json:"created"`
}

func (admin *securityAdmin) trustCertificate(w http.ResponseWriter, r *http.Request) {
	var request securityTrustImport
	if !decodeStrictSecurityRequest(w, r, &request) || rejectPrivateKeyMaterial(w, request.CertificatePEM) {
		return
	}
	kind, err := credentialstore.ParseKind(request.Kind)
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	if kind != credentialstore.KindTrustedAuthority && kind != credentialstore.KindPinnedPeer {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "kind must be trusted_authority or pinned_peer")
		return
	}
	certificate, err := admin.store().ImportCertificate(r.Context(), credentialstore.CertificateImport{
		Name:           request.Name,
		Kind:           kind,
		CertificatePEM: request.CertificatePEM,
		Source:         credentialstore.SourceMaster,
	})
	admin.finishCertificateWrite(w, r, certificate, false, err)
}

func (admin *securityAdmin) ensureClusterToken(w http.ResponseWriter, r *http.Request) {
	var request securityClusterTokenImport
	if !decodeSecurityRequest(w, r, &request) {
		return
	}
	token, created, err := admin.store().EnsureClusterToken(r.Context(), request.Name, request.Value)
	if err == nil && created {
		err = admin.refreshAfterWrite(r)
	}
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	openai.WriteJSON(w, status, securityClusterTokenResult{Token: token, Created: created})
}

func decodeStrictSecurityRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := strictJSONDecode(r.Body, target); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func strictJSONUnmarshal(content []byte, target any) error {
	return strictJSONDecode(bytes.NewReader(content), target)
}

func strictJSONDecode(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}
