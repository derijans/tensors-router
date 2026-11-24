package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/config"
	routerupdate "tensors-router/internal/update"
)

func runServe(args []string) error {
	arguments, cfg, err := loadServeConfig(args)
	if err != nil {
		return err
	}
	startupLogger, serveLogger := configuredLoggers(cfg.Logging.Mode)
	logStartupConfig(startupLogger, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownRequested := make(chan struct{}, 1)

	if err := routerupdate.NewManager(cfg).Ensure(ctx); err != nil {
		return err
	}

	runtime := &serveRuntime{cfg: cfg, arguments: arguments, startupLogger: startupLogger, logger: serveLogger}
	defer runtime.closeLogged()
	if err := runtime.startMCP(); err != nil {
		return err
	}
	discoveredModels, err := runtime.discoverModels()
	if err != nil {
		return err
	}
	if err := runtime.openAssetIndex(); err != nil {
		return err
	}
	if err := runtime.stores.open(ctx, cfg, serveLogger); err != nil {
		return err
	}
	if err := runtime.openCredentials(ctx); err != nil {
		return err
	}
	runtime.exportTrustBundle(ctx, arguments.trustBundlePath)
	runtime.exportAPICertificate(ctx, arguments.apiCertificateExport)
	if err := runtime.registerLocalModels(ctx, discoveredModels); err != nil {
		return err
	}
	runtime.connectCluster()
	runtime.startCompanions(ctx)
	syncConfig := runtime.clusterSyncConfig()
	runtime.syncConfiguredSlaves(ctx, syncConfig)
	runtime.locateFFmpeg()
	if err := runtime.createBackendFamilies(); err != nil {
		return err
	}
	authPolicy, err := runtime.newAuthPolicy()
	if err != nil {
		return err
	}
	runtime.startRouter(shutdownRequested)
	return runtime.serve(ctx, authPolicy, syncConfig, shutdownRequested)
}

func logStartupConfig(logger *log.Logger, cfg config.Config) {
	logger.Printf("tensors-router build %s", buildinfo.Current())
	logger.Printf("startup config profile=%s bind=%s node=%s role=%s backend=%s logging=%s", cfg.Security.Profile, cfg.Server.Bind, cfg.Cluster.NodeID, cfg.Cluster.Role, cfg.Backend.Mode, cfg.Logging.Mode)
	for _, warning := range cfg.Warnings {
		logger.Printf("configuration warning: %s", warning)
	}
}
