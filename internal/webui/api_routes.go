package webui

import (
	"net/http"
	"slices"
	"strings"
)

type apiHandler func(server *Server, w http.ResponseWriter, r *http.Request)

type apiRoute struct {
	path    string
	prefix  bool
	methods []string
	handle  apiHandler
}

func (route apiRoute) matches(r *http.Request) bool {
	if !slices.Contains(route.methods, r.Method) {
		return false
	}
	if route.prefix {
		return strings.HasPrefix(r.URL.Path, route.path)
	}
	return r.URL.Path == route.path
}

var apiRoutes = []apiRoute{
	handled("/api/session", (*Server).handleSession, http.MethodGet),
	handled("/api/logout", (*Server).logout, http.MethodPost),
	handled("/api/router/status", (*Server).writeRouterStatus, http.MethodGet),
	handled("/api/router/launch", routerAction("launch"), http.MethodPost),
	handled("/api/router/restart", routerAction("restart"), http.MethodPost),
	handled("/api/router/shutdown", routerAction("shutdown"), http.MethodPost),
	handled("/api/router/force-kill", routerAction("force-kill"), http.MethodPost),
	handled("/api/router/kill", routerAction("kill"), http.MethodPost),
	proxied("/api/inventory", "/router/v1/site/inventory", http.MethodGet),
	proxied("/api/nodes/state", "/router/v1/site/nodes/state", http.MethodGet),
	proxied("/api/nodes/unload", "/router/v1/site/nodes/unload", http.MethodPost),
	proxied("/api/nodes/backends/init", "/router/v1/site/nodes/backends/init", http.MethodPost),
	proxied("/api/nodes/backends/init/cancel", "/router/v1/site/nodes/backends/init/cancel", http.MethodPost),
	proxied("/api/nodes/backends/launch-options", "/router/v1/site/nodes/backends/launch-options", http.MethodGet, http.MethodPost),
	proxied("/api/models/state", "/router/v1/site/models/state", http.MethodPost),
	proxied("/api/routing-groups", "/router/v1/site/routing-groups", http.MethodGet, http.MethodPost, http.MethodDelete),
	proxied("/api/text-routing-groups", "/router/v1/site/text-routing-groups", http.MethodGet, http.MethodPost, http.MethodDelete),
	proxied("/api/separate-runtimes", "/router/v1/site/separate-runtimes", http.MethodGet, http.MethodPost),
	proxied("/api/download/capabilities", "/router/v1/site/download/capabilities", http.MethodGet),
	proxied("/api/download/search", "/router/v1/site/download/search", http.MethodPost),
	proxied("/api/download/search-page", "/router/v1/site/download/search-page", http.MethodPost),
	proxied("/api/download/repository", "/router/v1/site/download/repository", http.MethodPost),
	proxied("/api/download/plan", "/router/v1/site/download/plan", http.MethodPost),
	proxied("/api/download/jobs", "/router/v1/site/download/jobs", http.MethodPost),
	proxiedUnder("/api/download/jobs/", "/router/v1/site/download/jobs/", http.MethodGet, http.MethodPost),
	proxied("/api/download/library", "/router/v1/site/download/library", http.MethodGet),
	proxied("/api/download/rescan", "/router/v1/site/download/rescan", http.MethodPost),
	proxied("/api/webuis", "/router/v1/site/webuis", http.MethodGet),
	proxied("/api/webuis/session", "/router/v1/site/webuis/session", http.MethodPost),
	handled("/api/webuis/load", (*Server).proxyWebUILoad, http.MethodPost),
	proxied("/api/benchmarks", "/router/v1/benchmarks", http.MethodGet),
	proxied("/api/benchmarks/run", "/router/v1/benchmarks/run", http.MethodPost),
	proxied("/api/analytics", "/router/v1/site/analytics", http.MethodGet),
	proxied("/api/analytics/flush", "/router/v1/site/analytics/flush", http.MethodPost),
	proxied("/api/load-captures", "/router/v1/site/load-captures", http.MethodGet),
	proxiedUnder("/api/load-captures/", "/router/v1/site/load-captures/", http.MethodGet),
	proxied("/api/load-errors", "/router/v1/site/load-errors", http.MethodGet),
	proxied("/api/offload/settings", "/router/v1/site/offload/settings", http.MethodGet, http.MethodPost, http.MethodDelete),
	proxied("/api/offload/decisions", "/router/v1/site/offload/decisions", http.MethodGet),
	proxied("/api/offload/summary", "/router/v1/site/offload/summary", http.MethodGet),
	proxied("/api/load", "/router/v1/load", http.MethodPost),
	proxied("/api/cook/preview", "/router/v1/site/cook/preview", http.MethodPost),
	proxied("/api/cook/apply", "/router/v1/site/cook/apply", http.MethodPost),
	proxiedUnder("/api/cook/", "/router/v1/site/cook/", http.MethodDelete),
	proxied("/api/config-file/preview", "/router/v1/site/config-file/preview", http.MethodPost),
	proxied("/api/config-file/apply", "/router/v1/site/config-file/apply", http.MethodPost),
	proxied("/api/config-file", "/router/v1/site/config-file", http.MethodDelete),
	proxied("/api/model-assets/export", "/router/v1/site/model-assets/export", http.MethodPost),
	proxied("/api/model-files/hash", "/router/v1/site/model-files/hash", http.MethodPost),
	proxied("/api/model-assets/resolve", "/router/v1/site/model-assets/resolve", http.MethodPost),
	proxied("/api/model-assets/resolve-batch", "/router/v1/site/model-assets/resolve-batch", http.MethodPost),
	proxied("/api/model-assets/jobs", "/router/v1/site/model-assets/jobs", http.MethodPost),
	proxied("/api/model-assets/bind", "/router/v1/site/model-assets/bind", http.MethodPost),
	proxied("/api/model-assets/candidates", "/router/v1/site/model-assets/candidates", http.MethodPost),
	proxied("/api/model-assets/substitute", "/router/v1/site/model-assets/substitute", http.MethodPost),
	proxiedUnder("/api/model-assets/", "/router/v1/site/model-assets/", http.MethodGet),
}

func handled(path string, handle apiHandler, methods ...string) apiRoute {
	return apiRoute{path: path, methods: methods, handle: handle}
}

func proxied(path string, target string, methods ...string) apiRoute {
	return handled(path, func(server *Server, w http.ResponseWriter, r *http.Request) {
		server.proxyRouter(w, r, r.Method, target)
	}, methods...)
}

func proxiedUnder(prefix string, targetPrefix string, methods ...string) apiRoute {
	return apiRoute{path: prefix, prefix: true, methods: methods, handle: func(server *Server, w http.ResponseWriter, r *http.Request) {
		server.proxyRouter(w, r, r.Method, targetPrefix+strings.TrimPrefix(r.URL.Path, prefix))
	}}
}

func routerAction(action string) apiHandler {
	return func(server *Server, w http.ResponseWriter, r *http.Request) {
		server.handleRouterAction(w, r, action)
	}
}

func (server *Server) logout(w http.ResponseWriter, r *http.Request) {
	server.sessions.Logout(w, r)
	writeWebJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (server *Server) writeRouterStatus(w http.ResponseWriter, r *http.Request) {
	writeWebJSON(w, http.StatusOK, server.router.Status(r.Context()))
}
