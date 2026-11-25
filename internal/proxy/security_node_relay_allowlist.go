package proxy

import (
	"net/http"
	"strings"
)

const securityRelayWildcard = "*"

type securityRelayRoute struct {
	method  string
	pattern string
}

var securityRelayRoutes = []securityRelayRoute{
	{method: http.MethodGet, pattern: "summary"},
	{method: http.MethodPost, pattern: "csr"},
	{method: http.MethodDelete, pattern: "csr/*"},
	{method: http.MethodPost, pattern: "install"},
	{method: http.MethodPost, pattern: "certificates/generate"},
	{method: http.MethodPost, pattern: "certificates/*/activate"},
	{method: http.MethodDelete, pattern: "certificates/*"},
	{method: http.MethodGet, pattern: "certificates/*/pem"},
	{method: http.MethodPost, pattern: "tokens"},
	{method: http.MethodDelete, pattern: "tokens/*"},
	{method: http.MethodPost, pattern: "profile"},
}

func securityRelayAllowed(method string, suffix string) bool {
	for _, route := range securityRelayRoutes {
		if route.matches(method, suffix) {
			return true
		}
	}
	return false
}

func (route securityRelayRoute) matches(method string, suffix string) bool {
	if method != route.method {
		return false
	}
	expected := strings.Split(route.pattern, "/")
	actual := strings.Split(suffix, "/")
	if len(expected) != len(actual) {
		return false
	}
	for index, segment := range expected {
		if segment == securityRelayWildcard {
			if !relayableSegment(actual[index]) {
				return false
			}
			continue
		}
		if segment != actual[index] {
			return false
		}
	}
	return true
}

func relayableSegment(segment string) bool {
	return segment != "" && segment != "." && segment != ".." && !strings.ContainsAny(segment, "/?#")
}
