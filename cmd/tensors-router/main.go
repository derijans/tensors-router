package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/auth"
	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/buildinfo"
	"tensors-router/internal/catalog"
	routercluster "tensors-router/internal/cluster"
	"tensors-router/internal/config"
	"tensors-router/internal/kobold"
	"tensors-router/internal/native"
	"tensors-router/internal/proxy"
	"tensors-router/internal/recipes"
	"tensors-router/internal/transportbody"
	routerupdate "tensors-router/internal/update"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}

	switch args[0] {
	case "serve":
		return runServe(args[1:])
	case "download":
		return runDownload(args[1:])
	case "benchmark":
		return runBenchmark(args[1:])
	case "version":
		fmt.Println(buildinfo.Current())
		return nil
	case "-h", "--help", "help":
		return usage()
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runServe(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	configPath := flags.String("config", "config.yaml", "config file")
	securityProfile := flags.String("security-profile", "", "security profile")
	if err := flags.Parse(args); err != nil {
		return err
	}

	profileOverride := config.ResolveSecurityProfile(*securityProfile, os.Getenv("TENSORS_ROUTER_SECURITY_PROFILE"))
	cfg, err := config.LoadWithOptions(*configPath, config.LoadOptions{SecurityProfile: profileOverride})
	if err != nil {
		return err
	}
	startupLogger, serveLogger := configuredLoggers(cfg.Logging.Mode)
	startupLogger.Printf("tensors-router build %s", buildinfo.Current())
	startupLogger.Printf("startup config profile=%s bind=%s node=%s role=%s backend=%s logging=%s", cfg.Security.Profile, cfg.Server.Bind, cfg.Cluster.NodeID, cfg.Cluster.Role, cfg.Backend.Mode, cfg.Logging.Mode)
	for _, warning := range cfg.Warnings {
		startupLogger.Printf("configuration warning: %s", warning)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownRequested := make(chan struct{}, 1)

	updater := routerupdate.NewManager(cfg)
	if err := updater.Ensure(ctx); err != nil {
		return err
	}

	modelCatalog, err := catalog.NewWithStore(cfg.Models.ConfigDir, cfg.Cluster.StoreDir)
	if err != nil {
		return err
	}
	var analyticsStore *routeranalytics.Store
	var routerService *proxy.Service
	var shutdownBackends []func(context.Context) error
	runtimeCleaned := false
	cleanupRuntime := func() error {
		if runtimeCleaned {
			return nil
		}
		runtimeCleaned = true
		return closeRouterRuntime(routerService, modelCatalog, analyticsStore, shutdownBackends, serveLogger)
	}
	defer func() {
		if err := cleanupRuntime(); err != nil {
			serveLogger.Printf("runtime cleanup failed: %v", err)
		}
	}()
	benchmarkStore, err := routerbenchmark.NewStore(cfg.Cluster.StoreDir)
	if err != nil {
		return err
	}
	analyticsStore, err = newAnalyticsStore(cfg, serveLogger)
	if err != nil {
		return err
	}
	localModels, err := modelCatalog.List()
	if err != nil {
		return err
	}
	registry := routercluster.NewRegistry(cfg.Cluster.Role, cfg.Cluster.NodeID, cfg.Cluster.PublicURL)
	recipeStore, err := recipes.NewStore(cfg.Cluster.StoreDir)
	if err != nil {
		return err
	}
	localSource := routercluster.SourceLocal
	if cfg.Cluster.Role == routercluster.RoleMaster {
		localSource = routercluster.SourceMaster
	}
	if err := registry.UpdateLocal(withModelBenchmarks(routercluster.LocalModelsWithBackendMode(localModels, cfg.Cluster.NodeID, cfg.Cluster.PublicURL, localSource, cfg.Backend.Mode), benchmarkStore)); err != nil {
		return err
	}
	clusterClient := routercluster.NewClient(cfg.Cluster.Token, clusterClientTargets(cfg)...)
	syncConfig := routercluster.SyncConfig{
		Role:           cfg.Cluster.Role,
		MasterURL:      cfg.Cluster.MasterURL,
		SlaveURLs:      cfg.Cluster.SlaveURLs,
		SyncInterval:   cfg.Cluster.SyncInterval,
		HealthInterval: cfg.Cluster.HealthInterval,
	}
	routercluster.SyncConfiguredSlaves(ctx, syncConfig, registry, clusterClient, serveLogger)

	backendFamilies, backendShutdowns, err := createBackends(ctx, cfg)
	if err != nil {
		return err
	}
	shutdownBackends = backendShutdowns

	authPolicy, err := auth.NewPolicy(auth.PolicyConfig{
		AllowedCIDRs:  cfg.Server.AllowedCIDRs,
		Profile:       cfg.Security.Profile,
		InferenceKeys: cfg.Auth.InferenceKeys,
		AdminKeys:     cfg.Auth.AdminKeys,
		ClusterToken:  cfg.Cluster.Token,
	})
	if err != nil {
		return err
	}

	router := proxy.NewService(proxy.ServiceConfig{
		BackendMode:          cfg.Backend.Mode,
		BackendFamilies:      backendFamilies,
		Catalog:              modelCatalog,
		Registry:             registry,
		ClusterToken:         cfg.Cluster.Token,
		ClusterClient:        clusterClient,
		ClusterRole:          cfg.Cluster.Role,
		NodeID:               cfg.Cluster.NodeID,
		NodeURL:              cfg.Cluster.PublicURL,
		SlaveURLs:            cfg.Cluster.SlaveURLs,
		ConfigDir:            cfg.Models.ConfigDir,
		FileRoots:            cfg.Models.FileRoots,
		RecipeStore:          recipeStore,
		BenchmarkStore:       benchmarkStore,
		AnalyticsStore:       analyticsStore,
		VRAMAnalyticsEnabled: cfg.Analytics.Enabled && cfg.Analytics.VRAMEnabled,
		VRAMSampleInterval:   cfg.Analytics.VRAMSampleInterval,
		Logger:               serveLogger,
		Shutdown:             routerShutdownFunc(cfg, shutdownRequested),
		TransportLimits: transportbody.Limits{
			ReplayBufferBytes: cfg.Limits.ReplayBufferMB * transportbody.MiB,
			MemoryBudgetBytes: cfg.Limits.MemoryBudgetMB * transportbody.MiB,
			MaxRequestBytes:   cfg.Limits.MaxStreamRequestGB * transportbody.GiB,
			MaxResponseBytes:  cfg.Limits.MaxStreamResponseGB * transportbody.GiB,
			SelectorScanBytes: cfg.Limits.SelectorScanMB * transportbody.MiB,
		},
		MaxControlBodyBytes: cfg.Limits.MaxControlBodyMB * transportbody.MiB,
	})
	routerService = router
	routercluster.StartSync(ctx, syncConfig, registry, clusterClient, serveLogger)
	startupModel := strings.TrimSpace(cfg.Models.StartupModel)
	if startupModel != "" {
		serveLogger.Printf("startup model preload attempt model=%q", startupModel)
		if err := router.PreloadModel(ctx, startupModel); err != nil {
			return err
		}
		serveLogger.Printf("startup model preload succeeded model=%q", startupModel)
	}

	server := &http.Server{
		Addr:              cfg.Server.Bind,
		Handler:           authPolicy.Middleware(router),
		ReadHeaderTimeout: 15 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		startupLogger.Printf("listener ready address=%s", cfg.Server.Bind)
		errs <- server.ListenAndServe()
	}()

	var serveErr error
	select {
	case <-ctx.Done():
	case <-shutdownRequested:
	case listenerErr := <-errs:
		if !errors.Is(listenerErr, http.ErrServerClosed) {
			serveErr = listenerErr
		}
	}
	router.BeginDrain()
	drainErr := shutdownServer(server, cfg.Limits.DrainTimeout)
	cleanupErr := cleanupRuntime()
	return errors.Join(serveErr, drainErr, cleanupErr)
}

func shutdownServer(server *http.Server, timeout time.Duration) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		return errors.Join(err, server.Close())
	}
	return nil
}

func closeRouterRuntime(router *proxy.Service, modelCatalog *catalog.Catalog, analyticsStore *routeranalytics.Store, shutdownBackends []func(context.Context) error, logger *log.Logger) error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var closeErr error
	if analyticsStore != nil {
		closeErr = errors.Join(closeErr, analyticsStore.Close(shutdownContext))
	}
	if modelCatalog != nil {
		closeErr = errors.Join(closeErr, modelCatalog.Close())
	}
	if router != nil {
		closeErr = errors.Join(closeErr, router.Close(shutdownContext))
	}
	for _, shutdownBackend := range shutdownBackends {
		if err := shutdownBackend(shutdownContext); err != nil {
			logger.Printf("backend stop failed: %v", err)
			closeErr = errors.Join(closeErr, err)
		}
	}
	return closeErr
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

func configuredLoggers(mode string) (*log.Logger, *log.Logger) {
	discard := log.New(io.Discard, "", 0)
	switch mode {
	case config.LoggingModeNormal:
		return log.Default(), log.Default()
	case config.LoggingModeStartupOnly:
		return log.Default(), discard
	default:
		return discard, discard
	}
}

func bearerAuthConfigured(keys []string) bool {
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			return true
		}
	}
	return false
}

func newAnalyticsStore(cfg config.Config, logger *log.Logger) (*routeranalytics.Store, error) {
	if !cfg.Analytics.Enabled {
		return nil, nil
	}
	databasePath := strings.TrimSpace(cfg.Analytics.DatabasePath)
	if databasePath == "" {
		databasePath = filepath.Join(cfg.Cluster.StoreDir, "analytics.sqlite")
	}
	return routeranalytics.NewStore(routeranalytics.StoreConfig{
		NodeID:        cfg.Cluster.NodeID,
		DatabasePath:  databasePath,
		FlushInterval: cfg.Analytics.FlushInterval,
		RawRetention:  cfg.Analytics.RawRetention,
		Logger:        logger,
	})
}

func clusterClientTargets(cfg config.Config) []string {
	targets := []string{
		cfg.Cluster.PublicURL,
		cfg.Cluster.MasterURL,
	}
	targets = append(targets, cfg.Cluster.SlaveURLs...)
	return targets
}

func createBackends(ctx context.Context, cfg config.Config) (map[string]proxy.BackendFamilyConfig, []func(context.Context) error, error) {
	koboldManager, err := kobold.NewManager(koboldProcessConfig(cfg))
	if err != nil {
		return nil, nil, err
	}

	llamaManager, err := native.NewLlamaManager(llamaProcessConfig(cfg))
	if err != nil {
		return nil, nil, err
	}
	sdcppManager, err := native.NewSDCPPManager(sdcppProcessConfig(cfg))
	if err != nil {
		return nil, nil, err
	}

	if cfg.Backend.Mode != proxy.BackendModeLlamaSDCPP {
		if err := koboldManager.Start(ctx); err != nil {
			return nil, nil, err
		}
	}

	families := map[string]proxy.BackendFamilyConfig{
		proxy.BackendModeKobold: {
			TextBackend:  koboldManager,
			ImageBackend: koboldManager,
			Start:        koboldManager.Start,
			Stop:         koboldManager.Stop,
		},
		proxy.BackendModeLlamaSDCPP: {
			TextBackend:  llamaManager,
			ImageBackend: sdcppManager,
			Stop:         stopNativeManagers(llamaManager, sdcppManager),
		},
	}
	shutdownBackends := []func(context.Context) error{
		koboldManager.Stop,
		stopNativeManagers(llamaManager, sdcppManager),
	}
	return families, shutdownBackends, nil
}

func koboldProcessConfig(cfg config.Config) kobold.ProcessConfig {
	return kobold.ProcessConfig{
		BackendURL:   cfg.Kobold.BackendURL,
		BinaryPath:   cfg.Kobold.BinaryPath,
		ConfigDir:    cfg.Models.ConfigDir,
		DataDir:      cfg.Kobold.DataDir,
		ExtraArgs:    cfg.Kobold.ExtraArgs,
		Multiuser:    cfg.Kobold.Multiuser,
		Quiet:        cfg.Kobold.Quiet,
		SkipLauncher: cfg.Kobold.SkipLauncher,
		NoModel:      cfg.Kobold.NoModel,
		HideWindow:   cfg.Kobold.HideWindow,
		Logging:      cfg.Logging.BackendLogsToDisk,
	}
}

func llamaProcessConfig(cfg config.Config) native.ProcessConfig {
	return native.ProcessConfig{
		BackendURL: cfg.Llama.BackendURL,
		BinaryPath: cfg.Llama.BinaryPath,
		ConfigDir:  cfg.Models.ConfigDir,
		DataDir:    cfg.Llama.DataDir,
		ExtraArgs:  cfg.Llama.ExtraArgs,
		HideWindow: cfg.Llama.HideWindow,
		Logging:    cfg.Logging.BackendLogsToDisk,
	}
}

func sdcppProcessConfig(cfg config.Config) native.ProcessConfig {
	return native.ProcessConfig{
		BackendURL: cfg.SDCPP.BackendURL,
		BinaryPath: cfg.SDCPP.BinaryPath,
		ConfigDir:  cfg.Models.ConfigDir,
		DataDir:    cfg.SDCPP.DataDir,
		ExtraArgs:  cfg.SDCPP.ExtraArgs,
		HideWindow: cfg.SDCPP.HideWindow,
		Logging:    cfg.Logging.BackendLogsToDisk,
	}
}

func stopNativeManagers(llamaManager *native.Manager, sdcppManager *native.Manager) func(context.Context) error {
	return func(ctx context.Context) error {
		var firstErr error
		if err := llamaManager.Unload(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := sdcppManager.Unload(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
		return firstErr
	}
}

func runDownload(args []string) error {
	flags := flag.NewFlagSet("download", flag.ContinueOnError)
	configPath := flags.String("config", "config.yaml", "config file")
	securityProfile := flags.String("security-profile", "", "security profile")
	if err := flags.Parse(args); err != nil {
		return err
	}

	profileOverride := config.ResolveSecurityProfile(*securityProfile, os.Getenv("TENSORS_ROUTER_SECURITY_PROFILE"))
	cfg, err := config.LoadWithOptions(*configPath, config.LoadOptions{SecurityProfile: profileOverride})
	if err != nil {
		return err
	}

	updater := routerupdate.NewManager(cfg)
	paths, err := updater.DownloadedPaths(context.Background())
	if err != nil {
		return err
	}

	for _, path := range paths {
		fmt.Printf("downloaded %s\n", path)
	}
	return nil
}

func usage() error {
	fmt.Println("usage:")
	fmt.Println("  tensors-router serve --config config.yaml")
	fmt.Println("  tensors-router download --config config.yaml")
	fmt.Println("  tensors-router benchmark --model model-id")
	fmt.Println("  tensors-router version")
	return nil
}

func withModelBenchmarks(models []routercluster.Model, store *routerbenchmark.Store) []routercluster.Model {
	if store == nil {
		return models
	}
	keys := make([]routerbenchmark.ModelKey, len(models))
	for index := range models {
		keys[index] = routerbenchmark.ModelKey{NodeID: models[index].NodeID, ModelID: models[index].LocalID}
	}
	benchmarks := store.ModelBenchmarks(keys)
	for index, key := range keys {
		if benchmark, ok := benchmarks[key]; ok {
			models[index].Benchmark = &benchmark
		}
	}
	return models
}
