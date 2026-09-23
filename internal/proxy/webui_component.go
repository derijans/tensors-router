package proxy

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"

	"tensors-router/internal/cluster"
	"tensors-router/internal/proxy/routing"
)

type webUIDeps interface {
	acquireModelConfigForBackendMode(mode string, ctx context.Context, modelID string, configFilename string, readiness backendReadiness, force bool) (*backendRuntime, func(), bool, error)
	runtimeForBackendMode(mode string, readiness backendReadiness) (*backendRuntime, error)
	localBackendAvailableForRoute(ctx context.Context, mode string, readiness backendReadiness) bool
	loadLocalConfig(ctx context.Context, mode string, publicID string, filename string, readiness backendReadiness) error
	siteModels() []cluster.Model
	localClusterModels() ([]cluster.Model, error)
	resolveBackendMode(mode string) (string, error)
	writeProxyResponse(w http.ResponseWriter, response *http.Response, virtualModelID string, rewriteModel bool) error
	siteControlAllowed() bool
	remoteInventoryURLs() []string
	rejectModelLoadWhileDraining(w http.ResponseWriter) bool
	clusterIdentity() clusterIdentity
	nodeProxyToken() string
	httpClient() *http.Client
}

type webUIProxy struct {
	deps      webUIDeps
	analytics *requestAnalytics
	session   *webUISession
	routeMu   sync.Mutex
	snapshot  atomic.Pointer[webUIRouteSnapshot]
}

func newWebUIProxy(deps webUIDeps, analytics *requestAnalytics) *webUIProxy {
	return &webUIProxy{deps: deps, analytics: analytics, session: newWebUISession()}
}

func (webUI *webUIProxy) identity() clusterIdentity {
	return webUI.deps.clusterIdentity()
}

func (webUI *webUIProxy) routes() []routing.Route {
	return append([]routing.Route{
		routing.Exact(http.MethodGet, "/router/v1/site/webuis", webUI.handleSiteWebUIs),
		routing.Exact(http.MethodPost, "/router/v1/site/webuis/session", webUI.handleSiteWebUISession),
		routing.Exact(http.MethodPost, "/router/v1/site/webuis/load", webUI.handleSiteWebUILoad),
	}, routing.ClusterOnly([]routing.Route{
		routing.Exact(http.MethodGet, "/router/v1/node/site/webuis", webUI.handleNodeSiteWebUIs),
		routing.Exact(http.MethodPost, "/router/v1/node/site/webuis/load", webUI.handleNodeSiteWebUILoad),
		routing.Prefix(routing.AnyMethod, nodeWebUIProxyPrefix, webUI.handleNodeWebUIProxy),
	})...)
}
