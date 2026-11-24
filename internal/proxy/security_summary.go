package proxy

import (
	"context"
	"crypto/x509"
	"net/http"

	"tensors-router/internal/certgen"
	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
	"tensors-router/internal/siteapi"
)

type securitySummary struct {
	NodeID         string                        `json:"node_id"`
	Status         securityStatusResponse        `json:"status"`
	APICertificate siteapi.APICertificateState   `json:"api_certificate"`
	Tokens         []credentialstore.Token       `json:"tokens"`
	Certificates   []credentialstore.Certificate `json:"certificates"`
	PendingKeys    []credentialstore.PendingKey  `json:"pending_keys"`
	DefaultHosts   []string                      `json:"default_hosts"`
}

func (admin *securityAdmin) handleSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := admin.summary(r.Context())
	if err != nil {
		writeCredentialError(w, err)
		return
	}
	openai.WriteJSON(w, http.StatusOK, summary)
}

func (admin *securityAdmin) summary(ctx context.Context) (securitySummary, error) {
	store := admin.store()
	tokens, err := store.Tokens(ctx)
	if err != nil {
		return securitySummary{}, err
	}
	certificates, err := store.Certificates(ctx)
	if err != nil {
		return securitySummary{}, err
	}
	pendingKeys, err := store.PendingKeys(ctx)
	if err != nil {
		return securitySummary{}, err
	}
	return securitySummary{
		NodeID:         admin.nodeID,
		Status:         admin.status(),
		APICertificate: describeActiveAPICertificate(admin.credentials.Current().APICertificateLeaf, certificates),
		Tokens:         tokens,
		Certificates:   certificates,
		PendingKeys:    pendingKeys,
		DefaultHosts:   append([]string{}, admin.defaultIdentityHosts...),
	}, nil
}

func (admin *securityAdmin) reportedAPICertificate(ctx context.Context) *siteapi.APICertificateState {
	if admin == nil || admin.credentials == nil {
		return nil
	}
	certificates, err := admin.store().Certificates(ctx)
	if err != nil {
		return nil
	}
	state := describeActiveAPICertificate(admin.credentials.Current().APICertificateLeaf, certificates)
	return &state
}

func describeActiveAPICertificate(leaf *x509.Certificate, certificates []credentialstore.Certificate) siteapi.APICertificateState {
	if leaf == nil {
		return siteapi.APICertificateState{}
	}
	state := siteapi.APICertificateState{Present: true, NotAfter: leaf.NotAfter.UTC(), Fingerprint: certgen.Fingerprint(leaf)}
	for _, certificate := range certificates {
		if certificate.Kind == credentialstore.KindAPICertificate && certificate.Fingerprint == state.Fingerprint {
			state.Name = certificate.Name
			state.Source = certificate.Source
			break
		}
	}
	return state
}
