package proxy

import (
	"net/http"

	"tensors-router/internal/proxy/routing"
)

func (service *Service) siteRoutes() []routing.Route {
	return []routing.Route{
		routing.Exact(http.MethodGet, "/router/v1/site/inventory", service.handleSiteInventory),
		routing.Exact(http.MethodGet, "/router/v1/site/nodes/state", service.handleSiteNodeState),
		routing.Exact(http.MethodPost, "/router/v1/site/nodes/unload", service.handleSiteNodeUnload),
		routing.Exact(http.MethodPost, "/router/v1/site/nodes/backends/init", service.handleSiteBackendInitialization),
		routing.Exact(http.MethodPost, "/router/v1/site/nodes/backends/init/cancel", service.handleSiteBackendInitializationCancel),
		routing.Exact(http.MethodGet, "/router/v1/site/nodes/backends/launch-options", service.handleSiteBackendLaunchOptions),
		routing.Exact(http.MethodPost, "/router/v1/site/nodes/backends/launch-options", service.handleSiteBackendLaunchOptions),
		routing.Exact(http.MethodPost, "/router/v1/site/models/state", service.handleSiteModelState),
		routing.Exact(routing.AnyMethod, "/router/v1/site/routing-groups", service.handleSiteImageRoutingLinks),
		routing.Exact(routing.AnyMethod, "/router/v1/site/text-routing-groups", service.handleSiteTextRoutingLinks),
		routing.Exact(routing.AnyMethod, "/router/v1/site/separate-runtimes", service.handleSiteSeparateRuntimes),
		routing.Exact(http.MethodGet, "/router/v1/site/analytics", service.handleSiteAnalytics),
		routing.Exact(http.MethodPost, "/router/v1/site/analytics/flush", service.handleSiteAnalyticsFlush),
		routing.Exact(http.MethodGet, "/router/v1/site/load-captures", service.handleSiteLoadCaptures),
		routing.Prefix(http.MethodGet, "/router/v1/site/load-captures/", service.handleSiteLoadCaptureRecord),
		routing.Exact(http.MethodGet, "/router/v1/site/load-errors", service.handleSiteLoadErrors),
		routing.Exact(http.MethodPost, "/router/v1/site/cook/preview", service.handleSiteCookPreview),
		routing.Exact(http.MethodPost, "/router/v1/site/cook/apply", service.handleSiteCookApply),
		routing.Prefix(http.MethodDelete, "/router/v1/site/cook/", service.handleSiteCookDelete),
		routing.Exact(http.MethodPost, "/router/v1/site/config-file/preview", service.handleSiteConfigFilePreview),
		routing.Exact(http.MethodPost, "/router/v1/site/config-file/apply", service.handleSiteConfigFileApply),
		routing.Exact(http.MethodDelete, "/router/v1/site/config-file", service.handleSiteConfigFileDelete),
	}
}
