package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
	"tensors-router/internal/proxy/routing"
)

const securityNodesPath = securitySitePath + "/nodes/"

type securityRelayDeps interface {
	clusterMaster() bool
	remoteNodeURL(nodeID string) (string, bool)
	exchangeWithNode(ctx context.Context, method string, nodeURL string, path string, requestJSON []byte) (cluster.RawResponse, error)
}

type securityNodeRelay struct {
	deps        securityRelayDeps
	credentials *credentialstore.Live
}

func newSecurityNodeRelay(deps securityRelayDeps, credentials *credentialstore.Live) *securityNodeRelay {
	return &securityNodeRelay{deps: deps, credentials: credentials}
}

func (relay *securityNodeRelay) routes() []routing.Route {
	return []routing.Route{routing.Prefix(routing.AnyMethod, securityNodesPath, relay.handle)}
}

func (relay *securityNodeRelay) handle(w http.ResponseWriter, r *http.Request) {
	if !relay.deps.clusterMaster() || relay.credentials == nil {
		openai.WriteEndpointNotFound(w)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	nodeID, suffix, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, securityNodesPath), "/")
	nodeURL, known := relay.deps.remoteNodeURL(nodeID)
	if !known {
		openai.WriteError(w, http.StatusNotFound, "not_found", fmt.Sprintf("unknown node %q", nodeID))
		return
	}
	requestJSON, ok := readSecurityRelayBody(w, r)
	if !ok {
		return
	}
	node := securityNodeClient{deps: relay.deps, nodeURL: nodeURL}
	switch {
	case r.Method == http.MethodPost && suffix == "trust":
		relay.trust(w, r, node, requestJSON)
	case r.Method == http.MethodPost && suffix == "cluster-token":
		relay.pushClusterToken(w, r, node, requestJSON)
	case r.Method == http.MethodPost && suffix == "issue":
		relay.issue(w, r, node, requestJSON)
	case securityRelayAllowed(r.Method, suffix):
		node.relay(w, r, r.Method, suffix+forwardedSecurityQuery(r), requestJSON)
	default:
		openai.WriteEndpointNotFound(w)
	}
}

func (relay *securityNodeRelay) store() *credentialstore.Store {
	return relay.credentials.Store()
}

func forwardedSecurityQuery(r *http.Request) string {
	if r.URL.Query().Get("force") == "true" {
		return "?force=true"
	}
	return ""
}
