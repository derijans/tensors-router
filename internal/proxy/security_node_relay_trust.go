package proxy

import (
	"net/http"
	"strings"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
)

const defaultPushedClusterTokenName = "cluster"

type securityNodeTrustRequest struct {
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	CertificatePEM string `json:"certificate_pem"`
	CertificateID  string `json:"certificate_id"`
}

type securityNodeClusterTokenPush struct {
	Name string `json:"name"`
}

func (relay *securityNodeRelay) trust(w http.ResponseWriter, r *http.Request, node securityNodeClient, requestJSON []byte) {
	var request securityNodeTrustRequest
	if !decodeSecurityRelayBody(w, requestJSON, &request) {
		return
	}
	trust := securityTrustImport{Name: request.Name, Kind: request.Kind, CertificatePEM: request.CertificatePEM}
	if reference := strings.TrimSpace(request.CertificateID); reference != "" {
		local, ok := relay.localTrustAnchor(w, r, reference, request.Name)
		if !ok {
			return
		}
		trust = local
	}
	node.relayJSON(w, r, "trust", trust)
}

func (relay *securityNodeRelay) localTrustAnchor(w http.ResponseWriter, r *http.Request, reference string, name string) (securityTrustImport, bool) {
	certificates, err := relay.store().Certificates(r.Context())
	if err != nil {
		writeCredentialError(w, err)
		return securityTrustImport{}, false
	}
	anchor, found := certificateByReference(certificates, reference)
	if !found {
		writeCredentialError(w, credentialstore.ErrNotFound)
		return securityTrustImport{}, false
	}
	if anchor.Kind != credentialstore.KindAuthority && anchor.Kind != credentialstore.KindTrustedAuthority {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "certificate_id must name an authority or a trusted authority")
		return securityTrustImport{}, false
	}
	certificatePEM, err := relay.store().PublicCertificatePEM(r.Context(), anchor.ID)
	if err != nil {
		writeCredentialError(w, err)
		return securityTrustImport{}, false
	}
	if strings.TrimSpace(name) == "" {
		name = anchor.Name
	}
	return securityTrustImport{Name: name, Kind: string(credentialstore.KindTrustedAuthority), CertificatePEM: certificatePEM}, true
}

func certificateByReference(certificates []credentialstore.Certificate, reference string) (credentialstore.Certificate, bool) {
	for _, certificate := range certificates {
		if certificate.ID == reference || certificate.Name == reference {
			return certificate, true
		}
	}
	return credentialstore.Certificate{}, false
}

func (relay *securityNodeRelay) pushClusterToken(w http.ResponseWriter, r *http.Request, node securityNodeClient, requestJSON []byte) {
	var request securityNodeClusterTokenPush
	if !decodeSecurityRelayBody(w, requestJSON, &request) {
		return
	}
	secret := relay.credentials.OutboundClusterToken()
	if secret == "" {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", "this router has no cluster token to send")
		return
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		name = defaultPushedClusterTokenName
	}
	node.relayJSON(w, r, "cluster-token", securityClusterTokenImport{Name: name, Value: secret})
}
