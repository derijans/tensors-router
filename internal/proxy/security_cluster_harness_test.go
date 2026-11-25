package proxy

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/credentialstore"
)

const securityTestClusterToken = "security-test-cluster-token"

func openTestCredentials(t *testing.T) *credentialstore.Live {
	t.Helper()
	ctx := context.Background()
	store, err := credentialstore.Open(ctx, credentialstore.Config{Path: filepath.Join(t.TempDir(), credentialstore.Filename)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	live, err := credentialstore.NewLive(ctx, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	return live
}

func newSecurityClusterService(t *testing.T, role string, nodeID string, live *credentialstore.Live, registry *cluster.Registry) *Service {
	t.Helper()
	return newSecurityClusterServiceWith(t, role, nodeID, SecurityConfig{Credentials: live, DefaultIdentityHosts: []string{"127.0.0.1"}}, registry)
}

func newSecurityClusterServiceWith(t *testing.T, role string, nodeID string, security SecurityConfig, registry *cluster.Registry) *Service {
	t.Helper()
	backendServer := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(backendServer.Close)
	backendURL, err := url.Parse(backendServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	return NewService(ServiceConfig{
		Backend:      &fakeBackend{url: backendURL, healthy: true},
		Catalog:      catalog.New(t.TempDir()),
		Registry:     registry,
		ClusterToken: securityTestClusterToken,
		ClusterRole:  role,
		NodeID:       nodeID,
		Logger:       log.New(io.Discard, "", 0),
		Security:     security,
	})
}

type securityTestCluster struct {
	master      *Service
	masterLive  *credentialstore.Live
	slave       *Service
	slaveLive   *credentialstore.Live
	slaveServer *httptest.Server
}

func newSecurityTestCluster(t *testing.T) securityTestCluster {
	t.Helper()
	return newSecurityTestClusterWithSlaveProfile(t, SecurityProfileControl{})
}

func newSecurityTestClusterWithSlaveProfile(t *testing.T, slaveProfile SecurityProfileControl) securityTestCluster {
	t.Helper()
	slaveLive := openTestCredentials(t)
	slaveSecurity := SecurityConfig{Credentials: slaveLive, DefaultIdentityHosts: []string{"127.0.0.1"}, Profile: slaveProfile}
	slave := newSecurityClusterServiceWith(t, cluster.RoleSlave, "worker", slaveSecurity, nil)
	slaveServer := httptest.NewServer(slave)
	t.Cleanup(slaveServer.Close)
	masterLive := openTestCredentials(t)
	master := newSecurityClusterService(t, cluster.RoleMaster, "master", masterLive, registryWithNode(t, "worker", slaveServer.URL))
	return securityTestCluster{master: master, masterLive: masterLive, slave: slave, slaveLive: slaveLive, slaveServer: slaveServer}
}

func registryWithNode(t *testing.T, nodeID string, nodeURL string) *cluster.Registry {
	t.Helper()
	registry := cluster.NewRegistry(cluster.RoleMaster, "master", "")
	if err := registry.UpdateNode(cluster.Snapshot{ProtocolVersion: cluster.ProtocolVersion, NodeID: nodeID, NodeURL: nodeURL}); err != nil {
		t.Fatal(err)
	}
	return registry
}

func serveSecurityRequest(handler http.Handler, method string, path string, body string, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func masterRelayPath(suffix string) string {
	return securityNodesPath + "worker/" + suffix
}
