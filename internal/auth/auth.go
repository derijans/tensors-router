package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"tensors-router/internal/credential"
	"tensors-router/internal/ollama"
)

const (
	ProfileSecure     = "secure"
	ProfileTrustedLAN = "trusted_lan"
)

type PolicyConfig struct {
	AllowedCIDRs  []string
	Profile       string
	Keys          KeySource
	InferenceKeys []string
	AdminKeys     []string
	MetricsKeys   []string
	ClusterToken  string
}

type routeClass int

const (
	routeInference routeClass = iota
	routeAdmin
	routeMetrics
	routeCluster
)

const MetricsPath = "/router/v1/metrics"

type Policy struct {
	networks []*net.IPNet
	profile  string
	keys     KeySource
}

type Guard struct {
	policy *Policy
}

type Principal struct {
	Admin   bool
	Cluster bool
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
	keys := config.Keys
	if keys == nil {
		keys = staticKeySource(config)
	}
	return &Policy{networks: networks, profile: config.Profile, keys: keys}, nil
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
		keys := policy.keys.KeyHashes()
		authorization := r.Header.Get("Authorization")
		principal := PrincipalFor(authorization, keys)
		r = r.WithContext(context.WithValue(r.Context(), principalContextKey{}, principal))
		class := classifyRoute(r.URL.Path)
		if class == routeCluster && !ClusterPeerAccepted(r) {
			writeAccessError(w, r.URL.Path, http.StatusUnauthorized, "verified peer certificate required")
			return
		}
		if !policy.authorized(class, authorization, keys) {
			writeAccessError(w, r.URL.Path, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func ClusterPeerAccepted(r *http.Request) bool {
	return r.TLS == nil || len(r.TLS.VerifiedChains) > 0
}

func (policy *Policy) authorized(class routeClass, authorization string, keys credential.KeyHashes) bool {
	if class == routeCluster {
		return allowedBearer(authorization, keys.Cluster)
	}
	if policy.profile != ProfileSecure {
		return true
	}
	allowed := keysFor(class, keys)
	return len(allowed) == 0 || allowedBearer(authorization, allowed)
}

func keysFor(class routeClass, keys credential.KeyHashes) []credential.TokenHash {
	switch class {
	case routeAdmin:
		return keys.Admin
	case routeMetrics:
		return append(append([]credential.TokenHash{}, keys.Metrics...), keys.Admin...)
	default:
		return keys.Inference
	}
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

func IsClusterRoute(path string) bool {
	return strings.HasPrefix(path, "/router/v1/node/")
}

func classifyRoute(path string) routeClass {
	if path == "/router/mcp" {
		return routeAdmin
	}
	if path == MetricsPath {
		return routeMetrics
	}
	if IsClusterRoute(path) {
		return routeCluster
	}
	if strings.HasPrefix(path, "/router/v1/") ||
		strings.HasPrefix(path, "/router/webuis/") {
		return routeAdmin
	}
	return routeInference
}

func (policy *Policy) principal(authorization string) Principal {
	return PrincipalFor(authorization, policy.keys.KeyHashes())
}

func PrincipalFor(authorization string, keys credential.KeyHashes) Principal {
	return Principal{
		Admin:   allowedBearer(authorization, keys.Admin),
		Cluster: allowedBearer(authorization, keys.Cluster),
	}
}
