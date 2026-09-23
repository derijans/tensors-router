package proxy

import (
	"net/http"

	"tensors-router/internal/proxy/routing"
)

func (service *Service) nodeRoutes() []routing.Route {
	return routing.ClusterOnly([]routing.Route{
		routing.Exact(http.MethodGet, "/router/v1/node/models", service.handleNodeModels),
		routing.Exact(http.MethodPost, "/router/v1/node/mcp", service.handleNodeMCP),
		routing.Exact(http.MethodPost, "/router/v1/node/models/state", service.handleNodeModelState),
		routing.Exact(http.MethodGet, nodeSeparateRuntimesPath, service.handleNodeSeparateRuntimes),
		routing.Exact(http.MethodPost, nodeSeparateRuntimesPath, service.handleNodeSeparateRuntimes),
		routing.Exact(http.MethodGet, "/router/v1/node/state", service.handleNodeState),
		routing.Exact(http.MethodPost, "/router/v1/node/state/unload", service.handleNodeStateUnload),
		routing.Exact(http.MethodPost, "/router/v1/node/backends/init", service.handleNodeBackendInitialization),
		routing.Exact(http.MethodPost, "/router/v1/node/backends/init/cancel", service.handleNodeBackendInitializationCancel),
		routing.Exact(http.MethodGet, "/router/v1/node/backends/launch-options", service.handleNodeBackendLaunchOptions),
		routing.Exact(http.MethodPost, "/router/v1/node/backends/launch-options", service.handleNodeBackendLaunchOptions),
		routing.Prefix(routing.AnyMethod, "/router/v1/node/inference/", service.handleNodeInference),
		routing.Exact(http.MethodGet, "/router/v1/node/site/inventory", service.handleNodeSiteInventory),
		routing.Exact(http.MethodGet, "/router/v1/node/runtime-status", service.handleNodeRuntimeStatus),
		routing.Exact(http.MethodPost, "/router/v1/node/offload/grant", service.handleNodeOffloadGrant),
		routing.Exact(http.MethodPost, "/router/v1/node/offload/request", service.handleNodeOffloadRequest),
		routing.Exact(http.MethodPost, nodeRoutingLinksPath, service.handleNodeRoutingLinks),
		routing.Exact(http.MethodGet, "/router/v1/node/analytics", service.handleNodeAnalytics),
		routing.Exact(http.MethodPost, "/router/v1/node/analytics/flush", service.handleNodeAnalyticsFlush),
		routing.Exact(http.MethodGet, "/router/v1/node/load-captures", service.handleNodeLoadCaptures),
		routing.Prefix(http.MethodGet, "/router/v1/node/load-captures/", service.handleNodeLoadCaptureRecord),
		routing.Exact(http.MethodGet, "/router/v1/node/load-errors", service.handleNodeLoadErrors),
		routing.Exact(http.MethodPost, "/router/v1/node/site/configs", service.handleNodeSiteConfigs),
		routing.Exact(http.MethodPost, "/router/v1/node/site/config-file/preview", service.handleNodeConfigFilePreview),
		routing.Exact(http.MethodPost, "/router/v1/node/site/config-file/apply", service.handleNodeConfigFileApply),
		routing.Exact(http.MethodDelete, "/router/v1/node/site/config-file", service.handleNodeConfigFileDelete),
		routing.Exact(http.MethodPost, "/router/v1/node/register", service.handleNodeRegister),
		routing.Exact(http.MethodPost, "/router/v1/node/load", service.handleRouterLoad),
		routing.Exact(http.MethodPost, "/router/v1/node/unload", service.handleRouterUnload),
	})
}
