package main

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"tensors-router/internal/config"
)

func exportedTrustBundleExists(t *testing.T, cfg config.Config) bool {
	t.Helper()
	runtime := listenerTestRuntime(t, cfg, true)
	runtime.logger = log.New(io.Discard, "", 0)
	runtime.credentials.listeners = runtime.planListeners()
	path := filepath.Join(t.TempDir(), "router-trust.pem")
	runtime.exportTrustBundle(context.Background(), path)
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

func TestTrustBundleSkippedWhileServerBindServesPlainHTTP(t *testing.T) {
	for _, cfg := range []config.Config{
		listenerTestConfig(config.SecurityProfileTrustedLAN, "standalone", ""),
		listenerTestConfig(config.SecurityProfileTrustedLAN, "master", "127.0.0.1:8081"),
	} {
		if exportedTrustBundleExists(t, cfg) {
			t.Fatalf("role %s with cluster.bind %q serves plain HTTP on server.bind, so the managed WebUI must not get a trust bundle", cfg.Cluster.Role, cfg.Cluster.Bind)
		}
	}
}

func TestTrustBundleExportedWhileServerBindServesTLS(t *testing.T) {
	for _, cfg := range []config.Config{
		listenerTestConfig(config.SecurityProfileSecure, "standalone", ""),
		listenerTestConfig(config.SecurityProfileTrustedLAN, "master", ""),
	} {
		if !exportedTrustBundleExists(t, cfg) {
			t.Fatalf("role %s with profile %s serves HTTPS on server.bind, so the managed WebUI needs the trust bundle", cfg.Cluster.Role, cfg.Security.Profile)
		}
	}
}
