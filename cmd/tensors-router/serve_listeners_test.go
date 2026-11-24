package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tensors-router/internal/config"
	"tensors-router/internal/credentialstore"
	"tensors-router/internal/routertls"
)

func listenerTestRuntime(t *testing.T, cfg config.Config, activeIdentity bool) *serveRuntime {
	t.Helper()
	ctx := context.Background()
	store, err := credentialstore.Open(ctx, credentialstore.Config{Path: filepath.Join(t.TempDir(), credentialstore.Filename)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if activeIdentity {
		identity, err := store.GenerateIdentity(ctx, "node", []string{"127.0.0.1"})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Activate(ctx, identity.ID); err != nil {
			t.Fatal(err)
		}
	}
	live, err := credentialstore.NewLive(ctx, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	return &serveRuntime{cfg: cfg, credentials: serveCredentials{store: store, live: live, tls: routertls.NewRuntime(live)}}
}

func listenerTestConfig(profile string, role string, clusterBind string) config.Config {
	var cfg config.Config
	cfg.Security.Profile = profile
	cfg.Server.Bind = "127.0.0.1:8080"
	cfg.Cluster.Role = role
	cfg.Cluster.Bind = clusterBind
	return cfg
}

func TestTrustedLANAPIListenerStaysPlainWithoutAPICertificate(t *testing.T) {
	runtime := listenerTestRuntime(t, listenerTestConfig(config.SecurityProfileTrustedLAN, "standalone", ""), true)
	listeners := runtime.planListeners()
	if len(listeners) != 1 || listeners[0].role != routertls.APIListener || listeners[0].tls {
		t.Fatalf("listeners %#v; want one plain API listener", listeners)
	}
}

func TestSecureAPIListenerFallsBackToIdentity(t *testing.T) {
	runtime := listenerTestRuntime(t, listenerTestConfig(config.SecurityProfileSecure, "standalone", ""), true)
	if listeners := runtime.planListeners(); !listeners[0].tls {
		t.Fatalf("secure API listener with an active identity must serve HTTPS: %#v", listeners)
	}
}

func TestClusterBindSplitsPeerAndAPITraffic(t *testing.T) {
	runtime := listenerTestRuntime(t, listenerTestConfig(config.SecurityProfileTrustedLAN, "master", "127.0.0.1:8081"), true)
	listeners := runtime.planListeners()
	if len(listeners) != 2 || listeners[0].tls || !listeners[1].tls || listeners[1].role != routertls.ClusterListener {
		t.Fatalf("listeners %#v; want plain API listener and TLS cluster listener", listeners)
	}
	reached := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	cases := []struct {
		listener routerListener
		path     string
		want     int
	}{
		{listeners[0], "/router/v1/node/models", http.StatusNotFound},
		{listeners[0], "/v1/models", http.StatusNoContent},
		{listeners[1], "/router/v1/node/models", http.StatusNoContent},
		{listeners[1], "/router/v1/site/security/status", http.StatusNotFound},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		testCase.listener.routes(reached).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, testCase.path, nil))
		if recorder.Code != testCase.want {
			t.Fatalf("%s listener %s = %d, want %d", testCase.listener.name, testCase.path, recorder.Code, testCase.want)
		}
	}
}

func TestSharedListenerKeepsEveryRouteWithoutClusterBind(t *testing.T) {
	runtime := listenerTestRuntime(t, listenerTestConfig(config.SecurityProfileSecure, "slave", ""), true)
	listeners := runtime.planListeners()
	if len(listeners) != 1 || listeners[0].role != routertls.SharedListener || !listeners[0].tls {
		t.Fatalf("listeners %#v; want one TLS shared listener", listeners)
	}
	recorder := httptest.NewRecorder()
	listeners[0].routes(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/router/v1/node/models", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("shared listener refused a node route: %d", recorder.Code)
	}
}
