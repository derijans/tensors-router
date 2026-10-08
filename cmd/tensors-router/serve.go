package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/config"
	routerupdate "tensors-router/internal/update"
)

func runServe(args []string) error {
	configPath, cfg, err := loadServeConfig(args)
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

	runtime := &serveRuntime{cfg: cfg, configPath: configPath, startupLogger: startupLogger, logger: serveLogger}
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
	authPolicy, err := newAuthPolicy(cfg)
	if err != nil {
		return err
	}
	runtime.startRouter(shutdownRequested)
	return runtime.serve(ctx, authPolicy, syncConfig, shutdownRequested)
}

func loadServeConfig(args []string) (string, config.Config, error) {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := flags.String("config", "config.yaml", "config file")
	securityProfile := flags.String("security-profile", "", "security profile")
	if err := flags.Parse(args); err != nil {
		return "", config.Config{}, err
	}

	profileOverride := config.ResolveSecurityProfile(*securityProfile, os.Getenv("TENSORS_ROUTER_SECURITY_PROFILE"))
	loadOptions, err := environmentLoadOptions(profileOverride)
	if err != nil {
		return "", config.Config{}, err
	}
	cfg, err := config.LoadWithOptions(*configPath, loadOptions)
	return *configPath, cfg, err
}

func logStartupConfig(logger *log.Logger, cfg config.Config) {
	logger.Printf("tensors-router build %s", buildinfo.Current())
	logger.Printf("startup config profile=%s bind=%s node=%s role=%s backend=%s logging=%s", cfg.Security.Profile, cfg.Server.Bind, cfg.Cluster.NodeID, cfg.Cluster.Role, cfg.Backend.Mode, cfg.Logging.Mode)
	for _, warning := range cfg.Warnings {
		logger.Printf("configuration warning: %s", warning)
	}
}
