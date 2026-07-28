package webui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigParsesSeparateWebUIConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  bind: "0.0.0.0:9443"
  backend_ui_bind: "0.0.0.0:9444"
  backend_ui_public_url: "https://webui.local:9444"
  state_dir: "./state"
  cert_hosts:
    - "webui.local"
    - "172.81.90.24"
  admin_token: "secret"

router:
  url: "https://router.local:8080"
  token: "router-secret"
  binary_path: "./tensors-router"
  config_path: "./router.yaml"
  start_when_missing: false
  shutdown_with_webui: false
  args:
    - "--x"
    - "1"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Bind != "0.0.0.0:9443" || cfg.Server.AdminToken != "secret" {
		t.Fatalf("unexpected server config %#v", cfg.Server)
	}
	if cfg.Server.BackendUIBind != "0.0.0.0:9444" || cfg.Server.BackendUIPublicURL != "https://webui.local:9444" {
		t.Fatalf("unexpected backend UI config %#v", cfg.Server)
	}
	if !reflect.DeepEqual(cfg.Server.CertHosts, []string{"webui.local", "172.81.90.24"}) {
		t.Fatalf("unexpected cert hosts %#v", cfg.Server.CertHosts)
	}
	if cfg.Router.URL != "https://router.local:8080" || cfg.Router.Token != "router-secret" {
		t.Fatalf("unexpected router config %#v", cfg.Router)
	}
	if cfg.Router.StartWhenMissing || cfg.Router.ShutdownWithWebUI {
		t.Fatalf("unexpected router booleans %#v", cfg.Router)
	}
	if !reflect.DeepEqual(cfg.Router.Args, []string{"--x", "1"}) {
		t.Fatalf("unexpected args %#v", cfg.Router.Args)
	}
}

func TestLoadConfigRejectsMergedOrUnsafeBackendUIOrigin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  bind: "127.0.0.1:8443"
  backend_ui_bind: "127.0.0.1:8443"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path, dir); err == nil {
		t.Fatal("expected merged listener rejection")
	}
	if err := os.WriteFile(path, []byte(`
server:
  backend_ui_public_url: "http://user:pass@example.test/backend"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path, dir); err == nil {
		t.Fatal("expected unsafe backend UI public URL rejection")
	}
}

func TestRouterProcessIsManagedWhenURLMissing(t *testing.T) {
	process := NewRouterProcess(DefaultConfig(t.TempDir()).Router, t.TempDir())
	if !process.Managed() {
		t.Fatalf("expected missing router url to create managed process")
	}
}

func TestLoadConfigSecurityProfileOverrideHasPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.yaml")
	if err := os.WriteFile(path, []byte(`
security:
  profile: "secure"
server:
  bind: "0.0.0.0:8443"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path, dir); err == nil {
		t.Fatal("expected secure non-loopback admin token error")
	}
	cfg, err := LoadConfigWithOverrides(path, dir, ConfigOverrides{SecurityProfile: SecurityProfileTrustedLAN})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.Profile != SecurityProfileTrustedLAN || cfg.Router.SecurityProfile != SecurityProfileTrustedLAN {
		t.Fatalf("profile was not propagated %#v", cfg)
	}
}

func TestLoadConfigRejectsAdminPlaceholder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  admin_token: "change-me"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path, dir); err == nil {
		t.Fatal("expected placeholder rejection")
	}
}

func TestManagedRouterLaunchArgumentsPropagateProfileAuthoritatively(t *testing.T) {
	args := routerLaunchArguments(RouterConfig{
		ConfigPath:      "router.yaml",
		Args:            []string{"--x", "1"},
		SecurityProfile: SecurityProfileTrustedLAN,
	})
	want := []string{"serve", "--config", "router.yaml", "--x", "1", "--security-profile", "trusted_lan"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected launch arguments %#v", args)
	}
}

func TestResolveRouterLaunchConfigMakesPathsAbsoluteBeforeWorkingDirectoryChanges(t *testing.T) {
	workingDirectory := t.TempDir()
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDirectory) })

	config, err := resolveRouterLaunchConfig(RouterConfig{BinaryPath: "bin/tensors-router", ConfigPath: "config.yaml"})
	if err != nil {
		t.Fatal(err)
	}
	if config.BinaryPath != filepath.Join(workingDirectory, "bin", "tensors-router") {
		t.Fatalf("unexpected binary path %q", config.BinaryPath)
	}
	if config.ConfigPath != filepath.Join(workingDirectory, "config.yaml") {
		t.Fatalf("unexpected config path %q", config.ConfigPath)
	}
}

func TestWaitHealthyReportsManagedRouterExitImmediately(t *testing.T) {
	process := NewRouterProcess(RouterConfig{}, t.TempDir())
	waitDone := make(chan error, 1)
	waitDone <- fmt.Errorf("startup failed")
	close(waitDone)

	started := time.Now()
	err := process.waitHealthy(context.Background(), time.Second, waitDone)
	if err == nil || !strings.Contains(err.Error(), "startup failed") {
		t.Fatalf("unexpected startup result %v", err)
	}
	if time.Since(started) >= time.Second {
		t.Fatalf("router exit was hidden until the health timeout")
	}
}

func TestLoadConfigRejectsManagedProfileOverrideArgument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.yaml")
	if err := os.WriteFile(path, []byte(`
router:
  args: ["--security-profile=trusted_lan"]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path, dir); err == nil {
		t.Fatal("expected managed profile override rejection")
	}
}

func TestLoadConfigMapsLegacyLoggingEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.yaml")
	if err := os.WriteFile(path, []byte(`
logging:
  enabled: false
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Logging.Mode != LoggingModeQuiet || len(cfg.Warnings) != 1 {
		t.Fatalf("unexpected logging compatibility result %#v warnings=%#v", cfg.Logging, cfg.Warnings)
	}
}

func TestContainerWebUIExampleIsValid(t *testing.T) {
	if _, err := LoadConfig(filepath.Join("..", "..", "deploy", "config", "webui.yaml"), t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
