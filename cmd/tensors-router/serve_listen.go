package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"tensors-router/internal/auth"
	routercluster "tensors-router/internal/cluster"
	"tensors-router/internal/config"
)

func (runtime *serveRuntime) serve(ctx context.Context, authPolicy *auth.Policy, syncConfig routercluster.SyncConfig, shutdownRequested <-chan struct{}) error {
	router := runtime.router
	if err := routercluster.RegisterInitial(ctx, syncConfig, runtime.registry, runtime.clusterProbeClient, runtime.logger); err != nil {
		router.BeginDrain()
		return err
	}
	router.StartSchedulingRefresh(ctx)
	router.StartIdleUnloads(ctx)
	syncErrors := routercluster.StartSync(ctx, syncConfig, runtime.registry, runtime.clusterProbeClient, runtime.logger)

	servers, listenerErrors, err := runtime.startListeners(authPolicy.Middleware(router))
	if err != nil {
		router.BeginDrain()
		return err
	}
	runtime.backendStartup = startInBackground(ctx, runtime.logger, backendStartupSteps(runtime.cfg, runtime.backendFamilies, router), router.RecordLoadFailure)

	serveErr := awaitServeEnd(ctx, shutdownRequested, listenerErrors, syncErrors)
	router.BeginDrain()
	drainErr := shutdownServers(servers, runtime.cfg.Limits.DrainTimeout)
	cleanupErr := runtime.close()
	return errors.Join(serveErr, drainErr, cleanupErr)
}

func awaitServeEnd(ctx context.Context, shutdownRequested <-chan struct{}, listenerErrors <-chan error, syncErrors <-chan error) error {
	select {
	case <-ctx.Done():
	case <-shutdownRequested:
	case listenerErr := <-listenerErrors:
		if !errors.Is(listenerErr, http.ErrServerClosed) {
			return listenerErr
		}
	case syncErr := <-syncErrors:
		return syncErr
	}
	return nil
}

func (runtime *serveRuntime) startListeners(handler http.Handler) ([]*http.Server, <-chan error, error) {
	planned := runtime.credentials.listeners
	listenerErrors := make(chan error, len(planned))
	servers := make([]*http.Server, 0, len(planned))
	for _, plan := range planned {
		listener, err := net.Listen("tcp", plan.bind)
		if err != nil {
			return nil, nil, errors.Join(err, closeServers(servers))
		}
		server := &http.Server{Addr: plan.bind, Handler: plan.routes(handler), ReadHeaderTimeout: 15 * time.Second}
		servers = append(servers, server)
		runtime.startupLogger.Printf("listener ready name=%s address=%s tls=%t", plan.name, listener.Addr(), plan.tls)
		go func() {
			listenerErrors <- runtime.serveListener(server, listener, plan)
		}()
	}
	return servers, listenerErrors, nil
}

func closeServers(servers []*http.Server) error {
	var err error
	for _, server := range servers {
		err = errors.Join(err, server.Close())
	}
	return err
}

func shutdownServers(servers []*http.Server, timeout time.Duration) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var err error
	for _, server := range servers {
		if shutdownErr := server.Shutdown(shutdownContext); shutdownErr != nil {
			err = errors.Join(err, shutdownErr, server.Close())
		}
	}
	return err
}

func routerShutdownFunc(cfg config.Config, adminTokenStored bool, shutdownRequested chan<- struct{}) func() {
	if cfg.Security.Profile == config.SecurityProfileSecure && !adminTokenStored {
		return nil
	}
	return func() {
		select {
		case shutdownRequested <- struct{}{}:
		default:
		}
	}
}

func (runtime *serveRuntime) serveListener(server *http.Server, listener net.Listener, plan routerListener) error {
	if !plan.tls {
		return server.Serve(listener)
	}
	server.TLSConfig = runtime.credentials.tls.ListenerConfig(plan.role)
	return server.ServeTLS(listener, "", "")
}
