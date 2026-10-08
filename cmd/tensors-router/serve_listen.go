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
	syncErrors := routercluster.StartSync(ctx, syncConfig, runtime.registry, runtime.clusterProbeClient, runtime.logger)

	server := &http.Server{
		Addr:              runtime.cfg.Server.Bind,
		Handler:           authPolicy.Middleware(router),
		ReadHeaderTimeout: 15 * time.Second,
	}
	listener, err := net.Listen("tcp", runtime.cfg.Server.Bind)
	if err != nil {
		router.BeginDrain()
		return err
	}
	runtime.startupLogger.Printf("listener ready address=%s", listener.Addr())
	listenerErrors := make(chan error, 1)
	go func() {
		listenerErrors <- server.Serve(listener)
	}()
	runtime.backendStartup = startInBackground(ctx, runtime.logger, backendStartupSteps(runtime.cfg, runtime.backendFamilies, router), router.RecordLoadFailure)

	serveErr := awaitServeEnd(ctx, shutdownRequested, listenerErrors, syncErrors)
	router.BeginDrain()
	drainErr := shutdownServer(server, runtime.cfg.Limits.DrainTimeout)
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

func shutdownServer(server *http.Server, timeout time.Duration) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return errors.Join(err, server.Close())
	}
	return nil
}

func routerShutdownFunc(cfg config.Config, shutdownRequested chan<- struct{}) func() {
	if cfg.Security.Profile == config.SecurityProfileSecure && !bearerAuthConfigured(cfg.Auth.AdminKeys) {
		return nil
	}
	return func() {
		select {
		case shutdownRequested <- struct{}{}:
		default:
		}
	}
}
