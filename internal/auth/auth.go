package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"

	"tensors-router/internal/ollama"
)

const (
	ProfileSecure     = "secure"
	ProfileTrustedLAN = "trusted_lan"
)

type PolicyConfig struct {
	AllowedCIDRs  []string
	Profile       string
	InferenceKeys []string
	AdminKeys     []string
	ClusterToken  string
}

type routeClass int

const (
	routeInference routeClass = iota
	routeAdmin
	routeCluster
)

type Policy struct {
	networks      []*net.IPNet
	profile       string
	inferenceKeys []string
	adminKeys     []string
	clusterToken  string
}

type Guard struct {
	policy *Policy
}

type Principal struct {
	Admin bool
}

type principalContextKey struct{}

func NewPolicy(config PolicyConfig) (*Policy, error) {
	if config.Profile == "" {
		config.Profile = ProfileSecure
	}
	if config.Profile != ProfileSecure && config.Profile != ProfileTrustedLAN {
		return nil, fmt.Errorf("invalid security profile %q", config.Profile)
	}
	networks := make([]*net.IPNet, 0, len(config.AllowedCIDRs))
	for _, cidr := range config.AllowedCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		networks = append(networks, network)
	}
	return &Policy{
		networks:      networks,
		profile:       config.Profile,
		inferenceKeys: normalizedKeys(config.InferenceKeys),
		adminKeys:     normalizedKeys(config.AdminKeys),
		clusterToken:  strings.TrimSpace(config.ClusterToken),
	}, nil
}

func NewGuard(allowedCIDRs []string, bearerKeys []string) (*Guard, error) {
	policy, err := NewPolicy(PolicyConfig{
		AllowedCIDRs:  allowedCIDRs,
		Profile:       ProfileSecure,
		InferenceKeys: bearerKeys,
		AdminKeys:     bearerKeys,
	})
	if err != nil {
		return nil, err
	}
	return &Guard{policy: policy}, nil
}

func (policy *Policy) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !policy.allowedRemote(r.RemoteAddr) {
			writeAccessError(w, r.URL.Path, http.StatusForbidden, "forbidden")
			return
		}
		principal := policy.principal(r.Header.Get("Authorization"))
		r = r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal))
		class := classifyRoute(r.URL.Path)
		if class == routeCluster {
			if policy.clusterToken == "" || !allowedBearer(r.Header.Get("Authorization"), []string{policy.clusterToken}) {
				writeAccessError(w, r.URL.Path, http.StatusUnauthorized, "unauthorized")
				return
			}
		} else if policy.profile == ProfileSecure {
			keys := policy.inferenceKeys
			if class == routeAdmin {
				keys = policy.adminKeys
			}
			if len(keys) > 0 && !allowedBearer(r.Header.Get("Authorization"), keys) {
				writeAccessError(w, r.URL.Path, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeAccessError(w http.ResponseWriter, path string, status int, message string) {
	if _, ok := ollama.Method(path); ok {
		ollama.WriteError(w, status, message)
		return
	}
	http.Error(w, message, status)
}

func PrincipalFromContext(ctx context.Context) Principal {
	principal, _ := ctx.Value(principalContextKey{}).(Principal)
	return principal
}

func (policy *Policy) principal(header string) Principal {
	if allowedBearer(header, policy.adminKeys) {
		return Principal{Admin: true}
	}
	return Principal{}
}

func (guard *Guard) Middleware(next http.Handler) http.Handler {
	return guard.policy.Middleware(next)
}

func (policy *Policy) allowedRemote(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, network := range policy.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func classifyRoute(path string) routeClass {
	if path == "/router/mcp" {
		return routeAdmin
	}
	if strings.HasPrefix(path, "/router/v1/node/") {
		return routeCluster
	}
	if strings.HasPrefix(path, "/router/v1/") ||
		strings.HasPrefix(path, "/router/webuis/") {
		return routeAdmin
	}
	return routeInference
}

func allowedBearer(header string, keys []string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	matched := 0
	for _, key := range keys {
		matched |= subtle.ConstantTimeCompare([]byte(token), []byte(key))
	}
	return matched == 1
}

func normalizedKeys(values []string) []string {
	keys := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			keys = append(keys, value)
		}
	}
	return keys
}
