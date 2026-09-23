package downloads

import (
	"net/http"

	"tensors-router/internal/proxy/routing"
)

func (handlers *Handlers) Routes() []routing.Route {
	return append(handlers.siteRoutes(), routing.ClusterOnly(handlers.nodeRoutes())...)
}

func (handlers *Handlers) siteRoutes() []routing.Route {
	return []routing.Route{
		routing.Exact(http.MethodGet, "/router/v1/site/download/capabilities", handlers.SiteCapabilities),
		routing.Exact(http.MethodPost, "/router/v1/site/download/search", handlers.SiteSearch),
		routing.Exact(http.MethodPost, "/router/v1/site/download/search-page", handlers.SiteSearchPage),
		routing.Exact(http.MethodPost, "/router/v1/site/download/repository", handlers.SiteRepository),
		routing.Exact(http.MethodPost, "/router/v1/site/download/plan", handlers.SitePlan),
		routing.Exact(http.MethodPost, "/router/v1/site/download/jobs", handlers.SiteCreateJob),
		routing.PrefixSuffix(http.MethodGet, "/router/v1/site/download/jobs/", "/events", handlers.SiteEvents),
		routing.Prefix(http.MethodGet, "/router/v1/site/download/jobs/", handlers.SiteJob),
		routing.PrefixSuffix(http.MethodPost, "/router/v1/site/download/jobs/", "/pause", handlers.SitePause),
		routing.PrefixSuffix(http.MethodPost, "/router/v1/site/download/jobs/", "/resume", handlers.SiteResume),
		routing.PrefixSuffix(http.MethodPost, "/router/v1/site/download/jobs/", "/cancel", handlers.SiteCancel),
		routing.Exact(http.MethodGet, "/router/v1/site/download/library", handlers.SiteLibrary),
		routing.Exact(http.MethodPost, "/router/v1/site/download/rescan", handlers.SiteRescan),
	}
}

func (handlers *Handlers) nodeRoutes() []routing.Route {
	return []routing.Route{
		routing.Exact(http.MethodGet, "/router/v1/node/site/download/capabilities", handlers.NodeCapabilities),
		routing.Exact(http.MethodPost, "/router/v1/node/site/download/search", handlers.NodeSearch),
		routing.Exact(http.MethodPost, "/router/v1/node/site/download/search-page", handlers.NodeSearchPage),
		routing.Exact(http.MethodPost, "/router/v1/node/site/download/repository", handlers.NodeRepository),
		routing.Exact(http.MethodPost, "/router/v1/node/site/download/plan", handlers.NodePlan),
		routing.Exact(http.MethodPost, "/router/v1/node/site/download/jobs", handlers.NodeCreateJob),
		routing.PrefixSuffix(http.MethodGet, "/router/v1/node/site/download/jobs/", "/events", handlers.NodeEvents),
		routing.Prefix(http.MethodGet, "/router/v1/node/site/download/jobs/", handlers.NodeJob),
		routing.PrefixSuffix(http.MethodPost, "/router/v1/node/site/download/jobs/", "/pause", handlers.NodePause),
		routing.PrefixSuffix(http.MethodPost, "/router/v1/node/site/download/jobs/", "/resume", handlers.NodeResume),
		routing.PrefixSuffix(http.MethodPost, "/router/v1/node/site/download/jobs/", "/cancel", handlers.NodeCancel),
		routing.Exact(http.MethodGet, "/router/v1/node/site/download/library", handlers.NodeLibrary),
		routing.Exact(http.MethodPost, "/router/v1/node/site/download/rescan", handlers.NodeRescan),
	}
}
