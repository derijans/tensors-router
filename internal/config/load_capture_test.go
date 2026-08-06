package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCaptureConfigurationIsIndependentAndDisabledByDefault(t *testing.T) {
	defaults := Defaults()
	if defaults.Analytics.LoadCaptureEnabled || defaults.Analytics.LoadCaptureMaxOutputMB != 64 {
		t.Fatalf("unexpected load capture defaults: %#v", defaults.Analytics)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
logging:
  backend_logs_to_disk: false
analytics:
  enabled: false
  load_capture_enabled: true
  load_capture_database_path: "./private/captures.sqlite"
  load_capture_max_output_mb: 7
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Analytics.LoadCaptureEnabled || cfg.Analytics.Enabled || cfg.Logging.BackendLogsToDisk || cfg.Analytics.LoadCaptureDatabasePath != "./private/captures.sqlite" || cfg.Analytics.LoadCaptureMaxOutputMB != 7 {
		t.Fatalf("load capture configuration was not independent: %#v", cfg)
	}
}

func TestLoadRejectsInvalidLoadCaptureLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("analytics:\n  load_capture_max_output_mb: 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected invalid load capture limit error")
	}
}
