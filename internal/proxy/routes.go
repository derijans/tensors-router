package proxy

import (
	"fmt"
	"net/http"

	"tensors-router/internal/openai"
	"tensors-router/internal/proxy/routing"
)

type routeTable struct {
	exact               map[string]routing.Route
	prefixed            []routing.Route
	requireClusterToken func(http.ResponseWriter, *http.Request) bool
}

func exactRouteKey(method string, path string) string {
	return method + " " + path
}

func newRouteTable(requireClusterToken func(http.ResponseWriter, *http.Request) bool, groups ...[]routing.Route) *routeTable {
	table := &routeTable{exact: map[string]routing.Route{}, requireClusterToken: requireClusterToken}
	for _, group := range groups {
		for _, route := range group {
			table.add(route)
		}
	}
	return table
}

func (table *routeTable) add(route routing.Route) {
	if route.Prefix {
		table.prefixed = append(table.prefixed, route)
		return
	}
	key := exactRouteKey(route.Method, route.Path)
	if _, taken := table.exact[key]; taken {
		panic(fmt.Sprintf("route %s registered twice", key))
	}
	table.exact[key] = route
}

func (table *routeTable) match(method string, path string) (routing.Route, bool) {
	if found, ok := table.exact[exactRouteKey(method, path)]; ok {
		return found, true
	}
	if found, ok := table.exact[exactRouteKey(routing.AnyMethod, path)]; ok {
		return found, true
	}
	for _, route := range table.prefixed {
		if route.MatchesPrefixed(method, path) {
			return route, true
		}
	}
	return routing.Route{}, false
}

func (table *routeTable) serve(w http.ResponseWriter, r *http.Request) {
	matched, ok := table.match(r.Method, r.URL.Path)
	if !ok {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if matched.ClusterOnly && !table.requireClusterToken(w, r) {
		return
	}
	matched.Handler(w, r)
}
