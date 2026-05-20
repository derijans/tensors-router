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
	"syscall"
	"time"

	"tensors-router/internal/auth"
	"tensors-router/internal/catalog"
	"tensors-router/internal/config"
	"tensors-router/internal/kobold"
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

	modelCatalog := catalog.New(cfg.Models.ConfigDir)
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
		return err
	}

	if err := processManager.Start(ctx); err != nil {
		return err
	}
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := processManager.Stop(shutdownContext); err != nil {
			serveLogger.Printf("kobold stop failed: %v", err)
		}
	}()

	guard, err := auth.NewGuard(cfg.Server.AllowedCIDRs, cfg.Auth.BearerKeys)
	if err != nil {
		return err
	}

	router := proxy.NewService(proxy.ServiceConfig{
		Backend: processManager,
		Catalog: modelCatalog,
		Logger:  serveLogger,
	})

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
	if err := updater.Download(context.Background()); err != nil {
		return err
	}

	fmt.Printf("downloaded %s\n", cfg.Kobold.BinaryPath)
	return nil
}

func usage() error {
	fmt.Println("usage:")
	fmt.Println("  tensors-router serve --config config.yaml")
	fmt.Println("  tensors-router download --config config.yaml")
	fmt.Println("  tensors-router version")
	return nil
}
