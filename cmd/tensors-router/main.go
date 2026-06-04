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
	"strings"
	"syscall"
	"time"

	"tensors-router/internal/auth"
	"tensors-router/internal/catalog"
	routercluster "tensors-router/internal/cluster"
	"tensors-router/internal/config"
	"tensors-router/internal/kobold"
	"tensors-router/internal/native"
	"tensors-router/internal/proxy"
	routerupdate "tensors-router/internal/update"
)

const version = "0.1.0"

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
	case "version":
		fmt.Println(version)
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
	localModels, err := modelCatalog.List()
	if err != nil {
		return err
	}
	registry := routercluster.NewRegistry(cfg.Cluster.Role, cfg.Cluster.NodeID, cfg.Cluster.PublicURL)
	localSource := routercluster.SourceLocal
	if cfg.Cluster.Role == routercluster.RoleMaster {
		localSource = routercluster.SourceMaster
	}
	if err := registry.UpdateLocal(routercluster.LocalModelsWithBackendMode(localModels, cfg.Cluster.NodeID, cfg.Cluster.PublicURL, localSource, cfg.Backend.Mode)); err != nil {
		return err
	}
	clusterClient := routercluster.NewClient(cfg.Cluster.Token)
	syncConfig := routercluster.SyncConfig{
		Role:           cfg.Cluster.Role,
		MasterURL:      cfg.Cluster.MasterURL,
		SlaveURLs:      cfg.Cluster.SlaveURLs,
		SyncInterval:   cfg.Cluster.SyncInterval,
		HealthInterval: cfg.Cluster.HealthInterval,
	}
	routercluster.SyncConfiguredSlaves(ctx, syncConfig, registry, clusterClient, serveLogger)

	textBackend, imageBackend, shutdownBackends, err := createBackends(ctx, cfg)
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
		Backend:       textBackend,
		TextBackend:   textBackend,
		ImageBackend:  imageBackend,
		BackendMode:   cfg.Backend.Mode,
		Catalog:       modelCatalog,
		Registry:      registry,
		ClusterToken:  cfg.Cluster.Token,
		ClusterClient: clusterClient,
		Logger:        serveLogger,
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

func createBackends(ctx context.Context, cfg config.Config) (proxy.Backend, proxy.Backend, []func(context.Context) error, error) {
	switch cfg.Backend.Mode {
	case proxy.BackendModeLlamaSDCPP:
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
			return nil, nil, nil, err
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
			return nil, nil, nil, err
		}
		return llamaManager, sdcppManager, []func(context.Context) error{llamaManager.Unload, sdcppManager.Unload}, nil
	default:
		processManager, err := kobold.NewManager(kobold.ProcessConfig{
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
			return nil, nil, nil, err
		}
		if err := processManager.Start(ctx); err != nil {
			return nil, nil, nil, err
		}
		return processManager, processManager, []func(context.Context) error{processManager.Stop}, nil
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
	fmt.Println("  tensors-router version")
	return nil
}
