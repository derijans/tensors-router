package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

const routerDatabaseFilename = "analytics.sqlite"

var databasesKeptSeparate = []string{"model-assets.sqlite", "model-state.sqlite"}

func resolveDatabasePath(cfg *Config) {
	if strings.TrimSpace(cfg.Cluster.DatabasePath) != "" {
		return
	}
	cfg.Cluster.DatabasePath = filepath.Join(cfg.Cluster.StoreDir, routerDatabaseFilename)
}

func warnDeprecatedDatabasePaths(cfg *Config) {
	deprecated := []struct {
		key   string
		value string
	}{
		{"analytics.database_path", cfg.Analytics.DatabasePath},
		{"analytics.load_capture_database_path", cfg.Analytics.LoadCaptureDatabasePath},
		{"diagnostics.database_path", cfg.Diagnostics.DatabasePath},
	}
	for _, entry := range deprecated {
		if strings.TrimSpace(entry.value) == "" {
			continue
		}
		cfg.Warnings = append(cfg.Warnings, entry.key+" is deprecated; the router keeps one database at cluster.database_path and imports this file once")
	}
}

func validateDatabasePath(cluster ClusterConfig) error {
	path := strings.TrimSpace(cluster.DatabasePath)
	if path == "" {
		return nil
	}
	if strings.Contains(path, "?") {
		return fmt.Errorf("cluster.database_path must not contain a question mark")
	}
	for _, separate := range databasesKeptSeparate {
		if filepath.Clean(path) == filepath.Clean(filepath.Join(cluster.StoreDir, separate)) {
			return fmt.Errorf("cluster.database_path must not name %s, which stays a separate database", separate)
		}
	}
	return nil
}
