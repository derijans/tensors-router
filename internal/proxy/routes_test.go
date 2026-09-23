package proxy

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"tensors-router/internal/proxy/routing"
)

type expectedRoute struct {
	method      string
	path        string
	clusterOnly bool
	handler     http.HandlerFunc
}

func handlerName(handler http.HandlerFunc) string {
	return runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
}

func routeMethodsOverlap(first routing.Route, second routing.Route) bool {
	return first.Method == routing.AnyMethod || second.Method == routing.AnyMethod || first.Method == second.Method
}

func routerEndpointsBeforeTheRouteTable(service *Service) []expectedRoute {
	return []expectedRoute{
		{http.MethodGet, "/router/v1/vllm/" + "sample-id", false, service.handleVLLMAdmin},
		{http.MethodPost, "/router/v1/vllm/" + "sample-id", false, service.handleVLLMAdmin},
		{http.MethodDelete, "/router/v1/vllm/" + "sample-id", false, service.handleVLLMAdmin},
		{http.MethodGet, "/router/v1/site/inventory", false, service.handleSiteInventory},
		{http.MethodGet, "/router/v1/site/nodes/state", false, service.handleSiteNodeState},
		{http.MethodPost, "/router/v1/site/nodes/unload", false, service.handleSiteNodeUnload},
		{http.MethodPost, "/router/v1/site/nodes/backends/init", false, service.handleSiteBackendInitialization},
		{http.MethodPost, "/router/v1/site/nodes/backends/init/cancel", false, service.handleSiteBackendInitializationCancel},
		{http.MethodGet, "/router/v1/site/nodes/backends/launch-options", false, service.handleSiteBackendLaunchOptions},
		{http.MethodPost, "/router/v1/site/nodes/backends/launch-options", false, service.handleSiteBackendLaunchOptions},
		{http.MethodPost, "/router/v1/site/models/state", false, service.handleSiteModelState},
		{http.MethodGet, "/router/v1/site/routing-groups", false, service.handleSiteImageRoutingLinks},
		{http.MethodPost, "/router/v1/site/routing-groups", false, service.handleSiteImageRoutingLinks},
		{http.MethodDelete, "/router/v1/site/routing-groups", false, service.handleSiteImageRoutingLinks},
		{http.MethodGet, "/router/v1/site/text-routing-groups", false, service.handleSiteTextRoutingLinks},
		{http.MethodPost, "/router/v1/site/text-routing-groups", false, service.handleSiteTextRoutingLinks},
		{http.MethodDelete, "/router/v1/site/text-routing-groups", false, service.handleSiteTextRoutingLinks},
		{http.MethodGet, "/router/v1/site/separate-runtimes", false, service.handleSiteSeparateRuntimes},
		{http.MethodPost, "/router/v1/site/separate-runtimes", false, service.handleSiteSeparateRuntimes},
		{http.MethodDelete, "/router/v1/site/separate-runtimes", false, service.handleSiteSeparateRuntimes},
		{http.MethodGet, "/router/v1/site/download/capabilities", false, service.downloads.SiteCapabilities},
		{http.MethodPost, "/router/v1/site/download/search", false, service.downloads.SiteSearch},
		{http.MethodPost, "/router/v1/site/download/search-page", false, service.downloads.SiteSearchPage},
		{http.MethodPost, "/router/v1/site/download/repository", false, service.downloads.SiteRepository},
		{http.MethodPost, "/router/v1/site/download/plan", false, service.downloads.SitePlan},
		{http.MethodPost, "/router/v1/site/download/jobs", false, service.downloads.SiteCreateJob},
		{http.MethodGet, "/router/v1/site/download/jobs/" + "sample-id" + "/events", false, service.downloads.SiteEvents},
		{http.MethodGet, "/router/v1/site/download/jobs/" + "sample-id", false, service.downloads.SiteJob},
		{http.MethodPost, "/router/v1/site/download/jobs/" + "sample-id" + "/pause", false, service.downloads.SitePause},
		{http.MethodPost, "/router/v1/site/download/jobs/" + "sample-id" + "/resume", false, service.downloads.SiteResume},
		{http.MethodPost, "/router/v1/site/download/jobs/" + "sample-id" + "/cancel", false, service.downloads.SiteCancel},
		{http.MethodGet, "/router/v1/site/download/library", false, service.downloads.SiteLibrary},
		{http.MethodPost, "/router/v1/site/download/rescan", false, service.downloads.SiteRescan},
		{http.MethodGet, "/router/v1/site/webuis", false, service.handleSiteWebUIs},
		{http.MethodPost, "/router/v1/site/webuis/session", false, service.handleSiteWebUISession},
		{http.MethodPost, "/router/v1/site/webuis/load", false, service.handleSiteWebUILoad},
		{http.MethodGet, "/router/v1/benchmarks", false, service.benchmarks.handleBenchmarks},
		{http.MethodPost, "/router/v1/benchmarks/run", false, service.benchmarks.handleBenchmarkRun},
		{http.MethodGet, "/router/v1/site/analytics", false, service.handleSiteAnalytics},
		{http.MethodPost, "/router/v1/site/analytics/flush", false, service.handleSiteAnalyticsFlush},
		{http.MethodGet, "/router/v1/site/load-captures", false, service.handleSiteLoadCaptures},
		{http.MethodGet, "/router/v1/site/load-captures/" + "sample-id", false, service.handleSiteLoadCaptureRecord},
		{http.MethodGet, "/router/v1/site/load-errors", false, service.handleSiteLoadErrors},
		{http.MethodPost, "/router/v1/site/cook/preview", false, service.handleSiteCookPreview},
		{http.MethodPost, "/router/v1/site/cook/apply", false, service.handleSiteCookApply},
		{http.MethodDelete, "/router/v1/site/cook/" + "sample-id", false, service.handleSiteCookDelete},
		{http.MethodPost, "/router/v1/site/config-file/preview", false, service.handleSiteConfigFilePreview},
		{http.MethodPost, "/router/v1/site/config-file/apply", false, service.handleSiteConfigFileApply},
		{http.MethodDelete, "/router/v1/site/config-file", false, service.handleSiteConfigFileDelete},
		{http.MethodPost, "/router/v1/site/model-assets/export", false, service.handleSiteModelAssetExport},
		{http.MethodPost, "/router/v1/site/model-files/hash", false, service.handleSiteModelFileHash},
		{http.MethodPost, "/router/v1/site/model-assets/resolve", false, service.handleSiteModelAssetResolve},
		{http.MethodPost, "/router/v1/site/model-assets/resolve-batch", false, service.handleSiteModelAssetResolveBatch},
		{http.MethodPost, "/router/v1/site/model-assets/jobs", false, service.handleSiteModelAssetCreateJob},
		{http.MethodPost, "/router/v1/site/model-assets/bind", false, service.handleSiteModelAssetBinding},
		{http.MethodPost, "/router/v1/site/model-assets/candidates", false, service.handleSiteModelAssetCandidates},
		{http.MethodPost, "/router/v1/site/model-assets/substitute", false, service.handleSiteModelAssetSubstitution},
		{http.MethodGet, "/router/v1/site/model-assets/jobs/" + "sample-id", false, service.handleSiteModelAssetJob},
		{http.MethodGet, "/router/v1/site/model-assets/" + "sample-id", false, service.handleSiteModelAssetLookup},
		{http.MethodGet, "/router/v1/models", false, service.handleRouterModels},
		{http.MethodGet, "/router/v1/version", false, service.handleRouterVersion},
		{http.MethodGet, "/router/v1/node/models", true, service.handleNodeModels},
		{http.MethodPost, "/router/v1/node/mcp", true, service.handleNodeMCP},
		{http.MethodPost, "/router/v1/node/models/state", true, service.handleNodeModelState},
		{http.MethodGet, nodeSeparateRuntimesPath, true, service.handleNodeSeparateRuntimes},
		{http.MethodPost, nodeSeparateRuntimesPath, true, service.handleNodeSeparateRuntimes},
		{http.MethodGet, "/router/v1/node/state", true, service.handleNodeState},
		{http.MethodPost, "/router/v1/node/state/unload", true, service.handleNodeStateUnload},
		{http.MethodPost, "/router/v1/node/backends/init", true, service.handleNodeBackendInitialization},
		{http.MethodPost, "/router/v1/node/backends/init/cancel", true, service.handleNodeBackendInitializationCancel},
		{http.MethodGet, "/router/v1/node/backends/launch-options", true, service.handleNodeBackendLaunchOptions},
		{http.MethodPost, "/router/v1/node/backends/launch-options", true, service.handleNodeBackendLaunchOptions},
		{http.MethodPost, "/router/v1/node/site/model-assets/resolve", true, service.handleNodeModelAssetResolve},
		{http.MethodPost, "/router/v1/node/site/model-assets/export", true, service.handleNodeModelAssetExport},
		{http.MethodPost, "/router/v1/node/site/model-files/hash", true, service.handleNodeModelFileHash},
		{http.MethodPost, "/router/v1/node/site/model-assets/jobs", true, service.handleNodeModelAssetCreateJob},
		{http.MethodPost, "/router/v1/node/site/model-assets/bind", true, service.handleNodeModelAssetBinding},
		{http.MethodPost, "/router/v1/node/site/model-assets/candidates", true, service.handleNodeModelAssetCandidates},
		{http.MethodPost, "/router/v1/node/site/model-assets/substitute", true, service.handleNodeModelAssetSubstitution},
		{http.MethodGet, "/router/v1/node/site/model-assets/jobs/" + "sample-id", true, service.handleNodeModelAssetJob},
		{http.MethodPost, "/router/v1/node/assets/lookup", true, service.handleNodeAssetLookup},
		{http.MethodPost, "/router/v1/node/assets/lookup-cluster", true, service.handleNodeClusterAssetLookup},
		{http.MethodGet, "/router/v1/node/assets/" + "sample-id", true, service.handleNodeAssetStream},
		{http.MethodGet, "/router/v1/node/inference/" + "sample-id", true, service.handleNodeInference},
		{http.MethodPost, "/router/v1/node/inference/" + "sample-id", true, service.handleNodeInference},
		{http.MethodDelete, "/router/v1/node/inference/" + "sample-id", true, service.handleNodeInference},
		{http.MethodGet, "/router/v1/node/site/inventory", true, service.handleNodeSiteInventory},
		{http.MethodGet, "/router/v1/node/site/download/capabilities", true, service.downloads.NodeCapabilities},
		{http.MethodPost, "/router/v1/node/site/download/search", true, service.downloads.NodeSearch},
		{http.MethodPost, "/router/v1/node/site/download/search-page", true, service.downloads.NodeSearchPage},
		{http.MethodPost, "/router/v1/node/site/download/repository", true, service.downloads.NodeRepository},
		{http.MethodPost, "/router/v1/node/site/download/plan", true, service.downloads.NodePlan},
		{http.MethodPost, "/router/v1/node/site/download/jobs", true, service.downloads.NodeCreateJob},
		{http.MethodGet, "/router/v1/node/site/download/jobs/" + "sample-id" + "/events", true, service.downloads.NodeEvents},
		{http.MethodGet, "/router/v1/node/site/download/jobs/" + "sample-id", true, service.downloads.NodeJob},
		{http.MethodPost, "/router/v1/node/site/download/jobs/" + "sample-id" + "/pause", true, service.downloads.NodePause},
		{http.MethodPost, "/router/v1/node/site/download/jobs/" + "sample-id" + "/resume", true, service.downloads.NodeResume},
		{http.MethodPost, "/router/v1/node/site/download/jobs/" + "sample-id" + "/cancel", true, service.downloads.NodeCancel},
		{http.MethodGet, "/router/v1/node/site/download/library", true, service.downloads.NodeLibrary},
		{http.MethodPost, "/router/v1/node/site/download/rescan", true, service.downloads.NodeRescan},
		{http.MethodGet, "/router/v1/node/site/webuis", true, service.handleNodeSiteWebUIs},
		{http.MethodGet, "/router/v1/node/runtime-status", true, service.handleNodeRuntimeStatus},
		{http.MethodPost, "/router/v1/node/offload/grant", true, service.handleNodeOffloadGrant},
		{http.MethodPost, "/router/v1/node/offload/request", true, service.handleNodeOffloadRequest},
		{http.MethodPost, nodeRoutingLinksPath, true, service.handleNodeRoutingLinks},
		{http.MethodPost, "/router/v1/node/site/webuis/load", true, service.handleNodeSiteWebUILoad},
		{http.MethodGet, nodeWebUIProxyPrefix + "sample-id", true, service.handleNodeWebUIProxy},
		{http.MethodPost, nodeWebUIProxyPrefix + "sample-id", true, service.handleNodeWebUIProxy},
		{http.MethodDelete, nodeWebUIProxyPrefix + "sample-id", true, service.handleNodeWebUIProxy},
		{http.MethodGet, "/router/v1/node/benchmarks", true, service.benchmarks.handleNodeBenchmarks},
		{http.MethodPost, "/router/v1/node/benchmarks/run", true, service.benchmarks.handleNodeBenchmarkRun},
		{http.MethodGet, "/router/v1/node/analytics", true, service.handleNodeAnalytics},
		{http.MethodPost, "/router/v1/node/analytics/flush", true, service.handleNodeAnalyticsFlush},
		{http.MethodGet, "/router/v1/node/load-captures", true, service.handleNodeLoadCaptures},
		{http.MethodGet, "/router/v1/node/load-captures/" + "sample-id", true, service.handleNodeLoadCaptureRecord},
		{http.MethodGet, "/router/v1/node/load-errors", true, service.handleNodeLoadErrors},
		{http.MethodPost, "/router/v1/node/site/configs", true, service.handleNodeSiteConfigs},
		{http.MethodPost, "/router/v1/node/site/config-file/preview", true, service.handleNodeConfigFilePreview},
		{http.MethodPost, "/router/v1/node/site/config-file/apply", true, service.handleNodeConfigFileApply},
		{http.MethodDelete, "/router/v1/node/site/config-file", true, service.handleNodeConfigFileDelete},
		{http.MethodPost, "/router/v1/node/register", true, service.handleNodeRegister},
		{http.MethodPost, "/router/v1/node/load", true, service.handleRouterLoad},
		{http.MethodPost, "/router/v1/node/unload", true, service.handleRouterUnload},
		{http.MethodPost, "/router/v1/load", false, service.handleRouterLoad},
		{http.MethodPost, "/router/v1/unload", false, service.handleRouterUnload},
		{http.MethodPost, "/router/v1/shutdown", false, service.handleRouterShutdown},
	}
}

func TestEveryRouterEndpointReachesItsHandler(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.NotFoundHandler(), "a")

	for _, want := range routerEndpointsBeforeTheRouteTable(service) {
		matched, ok := service.routes.match(want.method, want.path)
		if !ok {
			t.Errorf("%s %s matched no route", want.method, want.path)
			continue
		}
		if got := handlerName(matched.Handler); got != handlerName(want.handler) {
			t.Errorf("%s %s reached %s, want %s", want.method, want.path, got, handlerName(want.handler))
		}
		if matched.ClusterOnly != want.clusterOnly {
			t.Errorf("%s %s clusterOnly = %v, want %v", want.method, want.path, matched.ClusterOnly, want.clusterOnly)
		}
	}
}

func TestNoExactRouteIsAlsoClaimedByAPrefixRoute(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.NotFoundHandler(), "a")

	for _, exact := range service.routes.exact {
		for _, prefixed := range service.routes.prefixed {
			if routeMethodsOverlap(exact, prefixed) && strings.HasPrefix(exact.Path, prefixed.Path) && strings.HasSuffix(exact.Path, prefixed.Suffix) {
				t.Errorf("exact route %s %s is also claimed by prefix route %s %s", exact.Method, exact.Path, prefixed.Method, prefixed.Path)
			}
		}
	}
}

func TestUnmatchedRouterEndpointsAreNotFound(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.NotFoundHandler(), "a")
	unmatched := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/router/v1/load"},
		{http.MethodDelete, "/router/v1/site/inventory"},
		{http.MethodPost, "/router/v1/site/download/jobs/sample-id"},
		{http.MethodPut, "/router/v1/site/nodes/backends/launch-options"},
		{http.MethodGet, "/router/v1/unknown"},
	}

	for _, request := range unmatched {
		recorder := httptest.NewRecorder()
		service.ServeHTTP(recorder, httptest.NewRequest(request.method, request.path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("%s %s status = %d, want %d", request.method, request.path, recorder.Code, http.StatusNotFound)
		}
	}
}

func TestClusterOnlyRouteRejectsARequestWithoutTheClusterToken(t *testing.T) {
	service, _ := newTestServiceWithModels(t, http.NotFoundHandler(), "a")
	service.clusterToken = "cluster-secret"

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/node/state", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
