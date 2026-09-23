package routing

import (
	"net/http"
	"strings"
)

const AnyMethod = "*"

type Route struct {
	Method      string
	Path        string
	Prefix      bool
	Suffix      string
	ClusterOnly bool
	Handler     http.HandlerFunc
}

func Exact(method string, path string, handler http.HandlerFunc) Route {
	return Route{Method: method, Path: path, Handler: handler}
}

func Prefix(method string, prefix string, handler http.HandlerFunc) Route {
	return Route{Method: method, Path: prefix, Prefix: true, Handler: handler}
}

func PrefixSuffix(method string, prefix string, suffix string, handler http.HandlerFunc) Route {
	return Route{Method: method, Path: prefix, Prefix: true, Suffix: suffix, Handler: handler}
}

func ClusterOnly(routes []Route) []Route {
	guarded := make([]Route, len(routes))
	for index, route := range routes {
		route.ClusterOnly = true
		guarded[index] = route
	}
	return guarded
}

func (route Route) AcceptsMethod(method string) bool {
	return route.Method == AnyMethod || route.Method == method
}

func (route Route) MatchesPrefixed(method string, path string) bool {
	return route.AcceptsMethod(method) && strings.HasPrefix(path, route.Path) && strings.HasSuffix(path, route.Suffix)
}
