package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

	"tensors-router/internal/buildinfo"
	"tensors-router/internal/webui"
)

const (
	webUIServerShutdownTimeout   = 16 * time.Minute
	managedRouterShutdownTimeout = 16*time.Minute + 30*time.Second
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) == 1 {
		switch args[0] {
		case "version", "--version", "-v":
			fmt.Println(buildinfo.Current())
			return nil
		}
	}
	flags := flag.NewFlagSet("tensor-router-webui", flag.ContinueOnError)
	configPath := flags.String("config", "webui.yaml", "webui config file")
	bind := flags.String("bind", "", "https bind address")
	routerURL := flags.String("router-url", "", "router url")
	routerToken := flags.String("router-token", "", "router bearer token")
	adminToken := flags.String("admin-token", "", "webui admin token")
	securityProfile := flags.String("security-profile", "", "security profile")
	if err := flags.Parse(args); err != nil {
		return err
	}

	executablePath, err := os.Executable()
	if err != nil {
		return err
	}
	executableDir := filepath.Dir(executablePath)
	profileOverride := webui.ResolveSecurityProfile(*securityProfile, os.Getenv("TENSORS_ROUTER_SECURITY_PROFILE"))
	adminTokenOverride := firstNonEmpty(*adminToken, os.Getenv("TENSORS_ROUTER_WEBUI_TOKEN"), os.Getenv("TENSOR_ROUTER_WEBUI_TOKEN"))
	cfg, err := webui.LoadConfigWithOverrides(*configPath, executableDir, webui.ConfigOverrides{
		SecurityProfile: profileOverride,
		Bind:            *bind,
		RouterURL:       *routerURL,
		RouterToken:     *routerToken,
		AdminToken:      adminTokenOverride,
	})
	if err != nil {
		return err
	}
	startupLogger, runtimeLogger := webUILoggers(cfg.Logging.Mode)
	startupLogger.Printf("tensor-router-webui build %s", buildinfo.Current())
	startupLogger.Printf("startup config profile=%s bind=%s router_managed=%t logging=%s", cfg.Security.Profile, cfg.Server.Bind, strings.TrimSpace(cfg.Router.URL) == "", cfg.Logging.Mode)
	for _, warning := range cfg.Warnings {
		startupLogger.Printf("configuration warning: %s", warning)
	}
	token := webUIToken(cfg)
	if token.generated {
		runtimeLogger.Printf("generated webui admin token: %s", token.value)
	}

	certFile, keyFile, err := webui.CertificateFiles(cfg.Server)
	if err != nil {
		return err
	}

	routerProcess := webui.NewRouterProcess(cfg.Router, executableDir)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := routerProcess.EnsureStarted(ctx); err != nil {
		runtimeLogger.Printf("router auto-launch failed: %v", err)
	}

	handler := webui.NewServer(cfg, routerProcess, webui.NewSessionManager(token.value))
	adminServer := webui.WebHTTPServer(cfg.Server.Bind, handler.AdminHandler())
	backendUIServer := webui.WebHTTPServer(cfg.Server.BackendUIBind, handler.BackendUIHandler())
	errs := make(chan error, 2)
	go func() {
		addr := webui.NormalizeBind(cfg.Server.Bind)
		startupLogger.Printf("admin listener ready address=https://%s", addr)
		errs <- adminServer.ListenAndServeTLS(certFile, keyFile)
	}()
	go func() {
		addr := webui.NormalizeBind(cfg.Server.BackendUIBind)
		startupLogger.Printf("backend UI listener ready address=https://%s", addr)
		errs <- backendUIServer.ListenAndServeTLS(certFile, keyFile)
	}()

	select {
	case <-ctx.Done():
		return shutdownWebUIRuntime(routerProcess, adminServer, backendUIServer)
	case listenerErr := <-errs:
		shutdownErr := shutdownWebUIRuntime(routerProcess, adminServer, backendUIServer)
		if listenerErr == nil || errors.Is(listenerErr, http.ErrServerClosed) {
			return shutdownErr
		}
		return errors.Join(listenerErr, shutdownErr)
	}
}

func shutdownWebUIRuntime(routerProcess *webui.RouterProcess, servers ...*http.Server) error {
	routerResult := make(chan error, 1)
	go func() {
		routerResult <- shutdownManagedRouter(routerProcess)
	}()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), webUIServerShutdownTimeout)
	defer cancel()
	serverResults := make(chan error, len(servers))
	for _, server := range servers {
		go func(server *http.Server) {
			if err := server.Shutdown(shutdownCtx); err != nil {
				serverResults <- errors.Join(err, server.Close())
				return
			}
			serverResults <- nil
		}(server)
	}
	var shutdownErr error
	for range servers {
		shutdownErr = errors.Join(shutdownErr, <-serverResults)
	}
	return errors.Join(shutdownErr, <-routerResult)
}

func shutdownManagedRouter(routerProcess *webui.RouterProcess) error {
	shutdownCtx, cancel := context.WithTimeout(context.Background(), managedRouterShutdownTimeout)
	defer cancel()
	return routerProcess.Shutdown(shutdownCtx)
}

type tokenValue struct {
	value     string
	generated bool
}

func webUIToken(cfg webui.Config) tokenValue {
	if cfg.Security.Profile == webui.SecurityProfileTrustedLAN {
		return tokenValue{}
	}
	if value := strings.TrimSpace(cfg.Server.AdminToken); value != "" {
		return tokenValue{value: value}
	}
	return tokenValue{value: randomToken(), generated: true}
}

func webUILoggers(mode string) (*log.Logger, *log.Logger) {
	discard := log.New(io.Discard, "", 0)
	switch mode {
	case webui.LoggingModeNormal:
		return log.Default(), log.Default()
	case webui.LoggingModeStartupOnly:
		return log.Default(), discard
	default:
		return discard, discard
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func randomToken() string {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("random token failed: %v", err))
	}
	return hex.EncodeToString(buffer)
}
