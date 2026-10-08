package main

import (
	"context"
	"errors"
	"log"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	routercluster "tensors-router/internal/cluster"
	"tensors-router/internal/config"
	"tensors-router/internal/downloader"
	"tensors-router/internal/ffmpeg"
	"tensors-router/internal/mcp"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/proxy"
	"tensors-router/internal/vllm"
)

type serveRuntime struct {
	cfg           config.Config
	configPath    string
	startupLogger *log.Logger
	logger        *log.Logger

	mcpReconciler *mcp.Reconciler
	mcpGateway    *mcp.Gateway
	catalog       *catalog.Catalog
	assetIndex    *modelassets.Index
	stores        serveStores
	registry      *routercluster.Registry

	clusterProbeClient *routercluster.Client
	clusterClient      *routercluster.Client

	downloader            downloader.Service
	downloaderCapability  downloader.Capability
	vllm                  vllm.Service
	vllmUnavailableReason string
	ffmpeg                ffmpeg.Tool

	backendFamilies  map[string]proxy.BackendFamilyConfig
	shutdownBackends []func(context.Context) error
	router           *proxy.Service
	backendStartup   *backgroundStartup
	closed           bool
}

func (runtime *serveRuntime) closeLogged() {
	if err := runtime.close(); err != nil {
		runtime.logger.Printf("runtime cleanup failed: %v", err)
	}
}

func (runtime *serveRuntime) close() error {
	if runtime.closed {
		return nil
	}
	runtime.closed = true
	if runtime.router == nil {
		_ = runtime.stores.modelState.Close()
	}
	runtime.backendStartup.Stop()
	runtimeErr := errors.Join(
		closeRouterRuntime(runtime.router, runtime.catalog, runtime.stores.analytics, runtime.shutdownBackends, runtime.logger),
		closeDownloader(runtime.downloader),
		closeVLLM(runtime.vllm),
	)
	closeErr := errors.Join(runtimeErr, runtime.stores.offloadDecisions.Close(), runtime.stores.handle.Close())
	if runtime.assetIndex != nil {
		runtime.assetIndex.Close()
	}
	return closeErr
}

func closeRouterRuntime(router *proxy.Service, modelCatalog *catalog.Catalog, analyticsStore *routeranalytics.Store, shutdownBackends []func(context.Context) error, logger *log.Logger) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var closeErr error
	if router != nil {
		closeErr = errors.Join(closeErr, router.Close(shutdownContext))
	}
	if modelCatalog != nil {
		closeErr = errors.Join(closeErr, modelCatalog.Close())
	}
	if analyticsStore != nil {
		closeErr = errors.Join(closeErr, analyticsStore.Close(shutdownContext))
	}
	for _, shutdownBackend := range shutdownBackends {
		if err := shutdownBackend(shutdownContext); err != nil {
			logger.Printf("backend stop failed: %v", err)
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
}
