package proxy

import (
	"net/http"
	"strings"
)

var backendHeaderAllowlist = map[string]struct{}{
	"accept":              {},
	"accept-encoding":     {},
	"accept-language":     {},
	"cache-control":       {},
	"content-encoding":    {},
	"content-language":    {},
	"content-type":        {},
	"if-match":            {},
	"if-modified-since":   {},
	"if-none-match":       {},
	"if-range":            {},
	"if-unmodified-since": {},
	"range":               {},
	"user-agent":          {},
	"x-request-id":        {},
}

func copyBackendHeaders(destination http.Header, source http.Header) {
	blocked := connectionHeaderNames(source)
	for key, values := range source {
		lower := strings.ToLower(key)
		if _, connected := blocked[lower]; connected {
			continue
		}
		if _, allowed := backendHeaderAllowlist[lower]; !allowed {
			continue
		}
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}

func copyClusterRequestHeaders(destination http.Header, source http.Header) {
	copyBackendHeaders(destination, source)
	if value := strings.TrimSpace(source.Get("X-Tensors-Model")); value != "" {
		destination.Set("X-Tensors-Model", value)
	}
}

func connectionHeaderNames(header http.Header) map[string]struct{} {
	blocked := map[string]struct{}{
		"connection":          {},
		"keep-alive":          {},
		"proxy-authenticate":  {},
		"proxy-authorization": {},
		"te":                  {},
		"trailer":             {},
		"transfer-encoding":   {},
		"upgrade":             {},
	}
	for _, value := range header.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
				blocked[name] = struct{}{}
			}
		}
	}
	return blocked
}
