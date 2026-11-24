package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadConfigText(t *testing.T, text string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestLoadRejectsPlaceholderCredentials(t *testing.T) {
	for _, placeholder := range []string{"secret", "Password", "your-admin-key", "replace-with-distinct-admin-key", "example-key-1234567890"} {
		_, err := loadConfigText(t, "auth:\n  admin_keys: [\""+placeholder+"\"]\n")
		if err == nil || !strings.Contains(err.Error(), "known placeholder") {
			t.Fatalf("placeholder %q was accepted: %v", placeholder, err)
		}
	}
}

func TestLoadWarnsAboutShortCredentialsWithoutRejectingThem(t *testing.T) {
	cfg, err := loadConfigText(t, "auth:\n  inference_keys: [\"short-key\"]\n  admin_keys: [\"admin-key-long-enough-01\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	if !anyContains(cfg.Warnings, "auth.inference_keys is shorter than 16 characters") {
		t.Fatalf("missing short key warning: %#v", cfg.Warnings)
	}
	if anyContains(cfg.Warnings, "auth.admin_keys is shorter") {
		t.Fatalf("long admin key was reported as short: %#v", cfg.Warnings)
	}
}

func TestStoredCredentialsWarnWhenSecureProfileHasNoTokens(t *testing.T) {
	cfg, err := loadConfigText(t, "server:\n  bind: \"127.0.0.1:8080\"\n")
	if err != nil {
		t.Fatal(err)
	}
	warnings, err := cfg.CheckStoredCredentials(StoredCredentials{})
	if err != nil {
		t.Fatal(err)
	}
	if !anyContains(warnings, "no inference or admin tokens exist") {
		t.Fatalf("missing open admin API warning: %#v", warnings)
	}

	cfg, err = loadConfigText(t, "security:\n  profile: \"trusted_lan\"\n")
	if err != nil {
		t.Fatal(err)
	}
	warnings, err = cfg.CheckStoredCredentials(StoredCredentials{})
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("trusted_lan was warned about missing tokens it does not use: %#v", warnings)
	}
}

func TestStoredCredentialsRequireAdminTokenForMCPInEveryProfile(t *testing.T) {
	for _, profile := range []string{SecurityProfileSecure, SecurityProfileTrustedLAN} {
		cfg, err := loadConfigText(t, "security:\n  profile: \""+profile+"\"\nmcp:\n  enabled: true\n")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cfg.CheckStoredCredentials(StoredCredentials{Inference: true}); err == nil || !strings.Contains(err.Error(), "admin token is required") {
			t.Fatalf("profile %s enabled MCP without an admin token: %v", profile, err)
		}
		if _, err := cfg.CheckStoredCredentials(StoredCredentials{Admin: true}); err != nil {
			t.Fatalf("profile %s rejected MCP with an admin token: %v", profile, err)
		}
	}
}

func TestStoredCredentialsRequireClusterTokenOutsideStandalone(t *testing.T) {
	cfg, err := loadConfigText(t, "cluster:\n  role: \"master\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.CheckStoredCredentials(StoredCredentials{}); err == nil || !strings.Contains(err.Error(), "cluster token is required") {
		t.Fatalf("master started without a cluster token: %v", err)
	}
	if _, err := cfg.CheckStoredCredentials(StoredCredentials{Cluster: true}); err != nil {
		t.Fatalf("master rejected a stored cluster token: %v", err)
	}
}
