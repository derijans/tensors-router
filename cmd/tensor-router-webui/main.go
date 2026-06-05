package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"tensors-router/internal/webui"
)

const version = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("tensor-router-webui", flag.ContinueOnError)
	configPath := flags.String("config", "webui.yaml", "webui config file")
	bind := flags.String("bind", "", "https bind address")
	routerURL := flags.String("router-url", "", "router url")
	routerToken := flags.String("router-token", "", "router bearer token")
	adminToken := flags.String("admin-token", "", "webui admin token")
	if err := flags.Parse(args); err != nil {
		return err
	}

	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	executableDir := filepath.Dir(executablePath)
	cfg, err := webui.LoadConfig(*configPath, executableDir)
	if err != nil {
		return err
	}
	applyFlagOverrides(&cfg, *bind, *routerURL, *routerToken, *adminToken)
	token := webUIToken(cfg)
	if token.generated {
		log.Printf("generated webui admin token: %s", token.value)
	}

	certFile, keyFile, err := webui.CertificateFiles(cfg.Server)
	if err != nil {
		return err
	}

	routerProcess := webui.NewRouterProcess(cfg.Router, executableDir)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := routerProcess.EnsureStarted(ctx); err != nil {
		log.Printf("router auto-launch failed: %v", err)
	}

	handler := webui.NewServer(cfg, routerProcess, webui.NewSessionManager(token.value))
	server := webui.WebHTTPServer(cfg.Server.Bind, handler)
	errs := make(chan error, 1)
	go func() {
		log.Printf("tensor-router-webui listening on https://%s", cfg.Server.Bind)
		errs <- server.ListenAndServeTLS(certFile, keyFile)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return routerProcess.Shutdown(shutdownCtx)
	case err := <-errs:
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = routerProcess.Shutdown(shutdownCtx)
		return err
	}
}

type tokenValue struct {
	value     string
	generated bool
}

func webUIToken(cfg webui.Config) tokenValue {
	for _, value := range []string{
		cfg.Server.AdminToken,
		os.Getenv("TENSORS_ROUTER_WEBUI_TOKEN"),
		os.Getenv("TENSOR_ROUTER_WEBUI_TOKEN"),
	} {
		value = strings.TrimSpace(value)
		if value != "" {
			return tokenValue{value: value}
		}
	}
	return tokenValue{value: randomToken(), generated: true}
}

func applyFlagOverrides(cfg *webui.Config, bind string, routerURL string, routerToken string, adminToken string) {
	if strings.TrimSpace(bind) != "" {
		cfg.Server.Bind = strings.TrimSpace(bind)
	}
	if strings.TrimSpace(routerURL) != "" {
		cfg.Router.URL = strings.TrimSpace(routerURL)
	}
	if strings.TrimSpace(routerToken) != "" {
		cfg.Router.Token = strings.TrimSpace(routerToken)
	}
	if strings.TrimSpace(adminToken) != "" {
		cfg.Server.AdminToken = strings.TrimSpace(adminToken)
	}
}

func randomToken() string {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("random token failed: %v", err))
	}
	return hex.EncodeToString(buffer)
}
