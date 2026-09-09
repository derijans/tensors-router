package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadConfigFromYAML(t *testing.T, content string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestDatabasePathDefaultsUnderStoreDir(t *testing.T) {
	cfg, err := loadConfigFromYAML(t, "cluster:\n  store_dir: \"./data/store\"\n")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("./data/store", "analytics.sqlite")
	if cfg.Cluster.DatabasePath != want {
		t.Fatalf("database path = %q, want %q", cfg.Cluster.DatabasePath, want)
	}
}

func TestDatabasePathOverrideIsKept(t *testing.T) {
	cfg, err := loadConfigFromYAML(t, "cluster:\n  store_dir: \"./data/store\"\n  database_path: \"/data/router.sqlite\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Cluster.DatabasePath != "/data/router.sqlite" {
		t.Fatalf("database path = %q, want %q", cfg.Cluster.DatabasePath, "/data/router.sqlite")
	}
}

func TestDatabasePathRejectsDatabasesKeptSeparate(t *testing.T) {
	for _, separate := range databasesKeptSeparate {
		_, err := loadConfigFromYAML(t, "cluster:\n  store_dir: \"./data/store\"\n  database_path: \"data/store/"+separate+"\"\n")
		if err == nil {
			t.Fatalf("%s was accepted as the router database path", separate)
		}
	}
}

func TestDatabasePathRejectsQueryMarker(t *testing.T) {
	if _, err := loadConfigFromYAML(t, "cluster:\n  database_path: \"./store/router.sqlite?_pragma=foreign_keys(0)\"\n"); err == nil {
		t.Fatal("a database path containing a question mark was accepted")
	}
}

func TestDeprecatedDatabasePathsStillParseAndWarnOnce(t *testing.T) {
	cfg, err := loadConfigFromYAML(t, `analytics:
  database_path: "./store/old-analytics.sqlite"
  load_capture_database_path: "./store/old-captures.sqlite"
diagnostics:
  database_path: "./store/old-errors.sqlite"
`)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Analytics.DatabasePath != "./store/old-analytics.sqlite" ||
		cfg.Analytics.LoadCaptureDatabasePath != "./store/old-captures.sqlite" ||
		cfg.Diagnostics.DatabasePath != "./store/old-errors.sqlite" {
		t.Fatalf("deprecated database paths were dropped: %#v", cfg)
	}
	for _, key := range []string{"analytics.database_path", "analytics.load_capture_database_path", "diagnostics.database_path"} {
		matches := 0
		for _, warning := range cfg.Warnings {
			if strings.HasPrefix(warning, key+" is deprecated") {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("%s produced %d warnings, want 1: %#v", key, matches, cfg.Warnings)
		}
	}
}

func TestEmptyDeprecatedDatabasePathsDoNotWarn(t *testing.T) {
	cfg, err := loadConfigFromYAML(t, "analytics:\n  database_path: \"\"\ndiagnostics:\n  database_path: \"\"\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range cfg.Warnings {
		if strings.Contains(warning, "database_path is deprecated") {
			t.Fatalf("empty deprecated key warned: %#v", cfg.Warnings)
		}
	}
}
