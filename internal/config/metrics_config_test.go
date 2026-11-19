package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsAPlaceholderMetricsKey(t *testing.T) {
	_, err := loadConfigText(t, "auth:\n  metrics_keys: [\"your-admin-key\"]\n")
	if err == nil || !strings.Contains(err.Error(), "known placeholder") {
		t.Fatalf("placeholder metrics key was accepted: %v", err)
	}
}

func TestLoadRejectsAMetricsKeyReusedAsAnAdminKey(t *testing.T) {
	shared := "d1c0f3a9b87e4c52a6e1f0b9c3d7a812"
	_, err := loadConfigText(t, "auth:\n  admin_keys: [\""+shared+"\"]\n  metrics_keys: [\""+shared+"\"]\n")
	if err == nil || !strings.Contains(err.Error(), "cannot be reused") {
		t.Fatalf("a key shared by admin and metrics was accepted: %v", err)
	}
}

func TestLoadReadsTheMetricsSection(t *testing.T) {
	cfg, err := loadConfigText(t, "metrics:\n  enabled: true\nauth:\n  metrics_keys: [\"7e2b9d04c6a14f3e8b5a0d1c9f6e3b27\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Metrics.Enabled || len(cfg.Auth.MetricsKeys) != 1 {
		t.Fatalf("metrics enabled=%t keys=%d", cfg.Metrics.Enabled, len(cfg.Auth.MetricsKeys))
	}
}

func TestMetricsAreDisabledByDefault(t *testing.T) {
	cfg, err := loadConfigText(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Metrics.Enabled {
		t.Fatal("metrics endpoint must be opt-in")
	}
}
