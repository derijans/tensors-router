package webui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadConfigParsesSeparateWebUIConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webui.yaml")
	if err := os.WriteFile(path, []byte(`
server:
  bind: "0.0.0.0:9443"
  state_dir: "./state"
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

func TestRouterProcessIsManagedWhenURLMissing(t *testing.T) {
	process := NewRouterProcess(DefaultConfig(t.TempDir()).Router, t.TempDir())
	if !process.Managed() {
		t.Fatalf("expected missing router url to create managed process")
	}
}
