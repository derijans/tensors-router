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
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	log.Printf("tensors-router build %s", buildinfo.Current())
	serveLogger := log.Default()
	if !cfg.Logging.Enabled {
		serveLogger = log.New(io.Discard, "", 0)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	updater := routerupdate.NewManager(cfg)
	if err := updater.Ensure(ctx); err != nil {
		return err
	}

	modelCatalog, err := catalog.NewWithStore(cfg.Models.ConfigDir, cfg.Cluster.StoreDir)
	if err != nil {
		return err
	}
	benchmarkStore, err := routerbenchmark.NewStore(cfg.Cluster.StoreDir)
	if err != nil {
		return err
	}
	analyticsStore, err := newAnalyticsStore(cfg, serveLogger)
	if err != nil {
		return err
	}
	if analyticsStore != nil {
		defer func() {
			shutdownContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := analyticsStore.Close(shutdownContext); err != nil {
				serveLogger.Printf("analytics shutdown failed: %v", err)
			}
		}()
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

	backendFamilies, shutdownBackends, err := createBackends(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		for _, shutdownBackend := range shutdownBackends {
			if err := shutdownBackend(shutdownContext); err != nil {
				serveLogger.Printf("backend stop failed: %v", err)
			}
		}
	}()

	bearerKeys := append([]string{}, cfg.Auth.BearerKeys...)
	if len(cfg.Auth.BearerKeys) > 0 && strings.TrimSpace(cfg.Cluster.Token) != "" {
		bearerKeys = append(bearerKeys, cfg.Cluster.Token)
	}
	guard, err := auth.NewGuard(cfg.Server.AllowedCIDRs, bearerKeys)
	if err != nil {
		return err
	}

	router := proxy.NewService(proxy.ServiceConfig{
		BackendMode:     cfg.Backend.Mode,
		BackendFamilies: backendFamilies,
		Catalog:         modelCatalog,
		Registry:        registry,
		ClusterToken:    cfg.Cluster.Token,
		ClusterClient:   clusterClient,
		ClusterRole:     cfg.Cluster.Role,
		NodeID:          cfg.Cluster.NodeID,
		NodeURL:         cfg.Cluster.PublicURL,
		SlaveURLs:       cfg.Cluster.SlaveURLs,
		ConfigDir:       cfg.Models.ConfigDir,
		FileRoots:       cfg.Models.FileRoots,
		RecipeStore:     recipeStore,
		BenchmarkStore:  benchmarkStore,
		AnalyticsStore:  analyticsStore,
		Logger:          serveLogger,
	})
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
		Handler:           guard.Middleware(router),
		ReadHeaderTimeout: 15 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		serveLogger.Printf("listening on %s", cfg.Server.Bind)
		errs <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return err
		}
		return nil
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
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
	koboldManager, err := kobold.NewManager(kobold.ProcessConfig{
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
		Logging:      cfg.Logging.Enabled,
	})
	if err != nil {
		return nil, nil, err
	}

	llamaManager, err := native.NewLlamaManager(native.ProcessConfig{
		BackendURL: cfg.Llama.BackendURL,
		BinaryPath: cfg.Llama.BinaryPath,
		ConfigDir:  cfg.Models.ConfigDir,
		DataDir:    cfg.Llama.DataDir,
		ExtraArgs:  cfg.Llama.ExtraArgs,
		HideWindow: cfg.Llama.HideWindow,
		Logging:    cfg.Logging.Enabled,
	})
	if err != nil {
		return nil, nil, err
	}
	sdcppManager, err := native.NewSDCPPManager(native.ProcessConfig{
		BackendURL: cfg.SDCPP.BackendURL,
		BinaryPath: cfg.SDCPP.BinaryPath,
		ConfigDir:  cfg.Models.ConfigDir,
		DataDir:    cfg.SDCPP.DataDir,
		ExtraArgs:  cfg.SDCPP.ExtraArgs,
		HideWindow: cfg.SDCPP.HideWindow,
		Logging:    cfg.Logging.Enabled,
	})
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
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
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
	for index := range models {
		benchmark, ok, err := store.ModelBenchmark(models[index].NodeID, models[index].LocalID)
		if err != nil || !ok {
			continue
		}
		models[index].Benchmark = &benchmark
	}
	return models
}
