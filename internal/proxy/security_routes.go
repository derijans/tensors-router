package proxy

import (
	"net/http"

	"tensors-router/internal/credentialstore"
	"tensors-router/internal/proxy/routing"
)

func (admin *securityAdmin) routes() []routing.Route {
	siteOnly := []routing.Route{
		routing.Exact(http.MethodGet, securityStatusPath, admin.handleStatus),
		routing.Exact(http.MethodPost, securityProfilePath, admin.handleSwitchToSecureProfile),
		routing.Exact(routing.AnyMethod, securityTokensPath, admin.handleTokens),
		routing.Prefix(routing.AnyMethod, securityTokensPath+"/", admin.handleToken),
		routing.Exact(routing.AnyMethod, securityCertificatesPath, admin.handleCertificates),
		routing.Prefix(routing.AnyMethod, securityCertificatesPath+"/", admin.handleCertificate(securityCertificatesPath)),
		routing.Exact(http.MethodPost, securityAuthoritiesPath, admin.handleCreateAuthority),
		routing.Prefix(http.MethodPost, securityAuthoritiesPath+"/", admin.handleIssueIdentity),
	}
	return admin.guardedRoutes(append(siteOnly, admin.certificateRequestRoutes(securitySitePath, credentialstore.SourceSupplied)...))
}

func (admin *securityAdmin) nodeRoutes() []routing.Route {
	tokensPath := securityNodePath + "/tokens"
	certificatesPath := securityNodePath + "/certificates"
	nodeOnly := []routing.Route{
		routing.Exact(http.MethodPost, securityNodePath+"/trust", admin.trustCertificate),
		routing.Exact(http.MethodPost, securityNodePath+"/cluster-token", admin.ensureClusterToken),
		routing.Exact(http.MethodPost, tokensPath, admin.createToken),
		routing.Prefix(http.MethodDelete, tokensPath+"/", admin.handleTokenDeletion(tokensPath)),
		routing.Prefix(routing.AnyMethod, certificatesPath+"/", admin.handleCertificate(certificatesPath)),
	}
	return routing.ClusterOnly(admin.guardedRoutes(append(nodeOnly, admin.certificateRequestRoutes(securityNodePath, credentialstore.SourceMaster)...)))
}

func (admin *securityAdmin) certificateRequestRoutes(rootPath string, installSource string) []routing.Route {
	requestsPath := rootPath + "/csr"
	return []routing.Route{
		routing.Exact(http.MethodGet, rootPath+"/summary", admin.handleSummary),
		routing.Exact(http.MethodPost, requestsPath, admin.createPendingKey),
		routing.Prefix(http.MethodDelete, requestsPath+"/", admin.handlePendingKeyDeletion(requestsPath)),
		routing.Exact(http.MethodPost, rootPath+"/install", admin.installSignedCertificate(installSource)),
	}
}

func (admin *securityAdmin) guardedRoutes(routes []routing.Route) []routing.Route {
	guarded := make([]routing.Route, len(routes))
	for index, route := range routes {
		route.Handler = admin.guarded(route.Handler)
		guarded[index] = route
	}
	return guarded
}
