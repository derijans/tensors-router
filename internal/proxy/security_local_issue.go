package proxy

import (
	"net/http"

	"tensors-router/internal/credentialstore"
)

const securityLocalIssuePath = securitySitePath + "/issue"

func (admin *securityAdmin) issueFromLocalAuthority(w http.ResponseWriter, r *http.Request) {
	var request securityNodeIssueRequest
	if !decodeStrictSecurityRequest(w, r, &request) {
		return
	}
	kind, ok := issuableKind(w, request)
	if !ok {
		return
	}
	hosts := request.Hosts
	if len(hosts) == 0 {
		hosts = admin.defaultIdentityHosts
	}
	store := admin.store()
	pending, err := store.CreatePendingKey(r.Context(), credentialstore.PendingKeyRequest{Name: request.Name, Kind: kind, Hosts: hosts})
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	chainPEM, err := store.SignCertificateRequest(r.Context(), request.AuthorityID, kind, pending.RequestPEM)
	if err != nil {
		_ = store.DeletePendingKey(r.Context(), pending.ID)
		writeCredentialError(w, err)
		return
	}
	certificate, err := store.InstallSignedCertificate(r.Context(), credentialstore.SignedCertificateInstall{
		CertificatePEM: chainPEM,
		Activate:       request.Activate,
		Source:         credentialstore.SourceIssued,
	})
	if err != nil {
		_ = store.DeletePendingKey(r.Context(), pending.ID)
	}
	admin.finishCertificateWrite(w, r, certificate, false, err)
}
