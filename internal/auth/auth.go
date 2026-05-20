package auth

import (
	"net"
	"net/http"
	"strings"
)

type Guard struct {
	networks   []*net.IPNet
	bearerKeys map[string]struct{}
}

func NewGuard(allowedCIDRs []string, bearerKeys []string) (*Guard, error) {
	networks := make([]*net.IPNet, 0, len(allowedCIDRs))
	for _, cidr := range allowedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		networks = append(networks, network)
	}

	keys := make(map[string]struct{}, len(bearerKeys))
	for _, key := range bearerKeys {
		if key != "" {
			keys[key] = struct{}{}
		}
	}

	return &Guard{
		networks:   networks,
		bearerKeys: keys,
	}, nil
}

func (guard *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !guard.allowedRemote(r.RemoteAddr) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if len(guard.bearerKeys) > 0 && !guard.allowedBearer(r.Header.Get("Authorization")) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (guard *Guard) allowedRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, network := range guard.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (guard *Guard) allowedBearer(header string) bool {
	prefix := "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	_, ok := guard.bearerKeys[strings.TrimSpace(strings.TrimPrefix(header, prefix))]
	return ok
}
