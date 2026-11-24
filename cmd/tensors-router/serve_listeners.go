package main

import (
	"net/http"

	"tensors-router/internal/auth"
	"tensors-router/internal/config"
	"tensors-router/internal/credentialstore"
	"tensors-router/internal/openai"
	"tensors-router/internal/proxy"
	"tensors-router/internal/routertls"
)

type routerListener struct {
	name          string
	bind          string
	role          routertls.ListenerRole
	tls           bool
	splitsCluster bool
}

func (runtime *serveRuntime) planListeners() []routerListener {
	cfg := runtime.cfg
	if cfg.SeparateClusterListener() {
		listeners := []routerListener{
			runtime.plannedListener("api", cfg.Server.Bind, routertls.APIListener),
			runtime.plannedListener("cluster", cfg.Cluster.Bind, routertls.ClusterListener),
		}
		for index := range listeners {
			listeners[index].splitsCluster = true
		}
		return listeners
	}
	if cfg.Cluster.Role == "standalone" {
		return []routerListener{runtime.plannedListener("api", cfg.Server.Bind, routertls.APIListener)}
	}
	return []routerListener{runtime.plannedListener("shared", cfg.Server.Bind, routertls.SharedListener)}
}

func (runtime *serveRuntime) plannedListener(name string, bind string, role routertls.ListenerRole) routerListener {
	return routerListener{name: name, bind: bind, role: role, tls: runtime.listenerWantsTLS(role)}
}

func (runtime *serveRuntime) listenerWantsTLS(role routertls.ListenerRole) bool {
	if role == routertls.APIListener {
		return runtime.credentials.tls.APIListenerServesTLS(runtime.cfg.Security.Profile == config.SecurityProfileSecure)
	}
	return runtime.credentials.tls.Enabled()
}

func (runtime *serveRuntime) peerListenerTLS() bool {
	for _, listener := range runtime.credentials.listeners {
		if listener.role != routertls.APIListener {
			return listener.tls
		}
	}
	return false
}

func (runtime *serveRuntime) serverBindListenerTLS() bool {
	for _, listener := range runtime.credentials.listeners {
		if listener.role != routertls.ClusterListener {
			return listener.tls
		}
	}
	return false
}

func (runtime *serveRuntime) securityListeners() []proxy.SecurityListener {
	listeners := make([]proxy.SecurityListener, 0, len(runtime.credentials.listeners))
	for _, listener := range runtime.credentials.listeners {
		role := listener.role
		listeners = append(listeners, proxy.SecurityListener{
			Name:     listener.name,
			Bind:     listener.bind,
			TLS:      listener.tls,
			WantsTLS: func() bool { return runtime.listenerWantsTLS(role) },
		})
	}
	return listeners
}

func (listener routerListener) routes(handler http.Handler) http.Handler {
	if !listener.splitsCluster {
		return handler
	}
	servesClusterRoutes := listener.role == routertls.ClusterListener
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.IsClusterRoute(r.URL.Path) != servesClusterRoutes {
			openai.WriteEndpointNotFound(w)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

func unservedAPICertificateWarnings(cfg config.Config, snapshot *credentialstore.Snapshot) []string {
	if snapshot.APICertificate == nil || cfg.SeparateClusterListener() || cfg.Cluster.Role == "standalone" {
		return nil
	}
	return []string{"an API certificate is active but cluster peers share server.bind, so the router serves its identity there; set cluster.bind to give peers their own listener and serve the API certificate on server.bind"}
}
