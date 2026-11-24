package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

func validateClusterRuntime(cfg *Config) error {
	switch cfg.Cluster.Role {
	case "standalone", "master", "slave":
	default:
		return fmt.Errorf("cluster.role must be standalone, master, or slave")
	}
	if strings.TrimSpace(cfg.Cluster.NodeID) == "" {
		return fmt.Errorf("cluster.node_id is required")
	}
	if cfg.Cluster.StoreDir == "" {
		return fmt.Errorf("cluster.store_dir is required")
	}
	if err := validateDatabasePath(cfg.Cluster); err != nil {
		return err
	}
	if cfg.Cluster.SyncInterval <= 0 {
		return fmt.Errorf("cluster.sync_interval must be positive")
	}
	if cfg.Cluster.HealthInterval <= 0 {
		return fmt.Errorf("cluster.health_interval must be positive")
	}
	if cfg.Cluster.ControlTimeout <= 0 {
		return fmt.Errorf("cluster.control_timeout must be positive")
	}
	if cfg.Cluster.SyncConcurrency <= 0 || cfg.Cluster.SyncConcurrency > 128 {
		return fmt.Errorf("cluster.sync_concurrency must be between 1 and 128")
	}
	return nil
}

func validateClusterBind(cfg *Config) error {
	bind := strings.TrimSpace(cfg.Cluster.Bind)
	if bind == "" {
		return nil
	}
	if cfg.Cluster.Role == "standalone" {
		return fmt.Errorf("cluster.bind needs cluster.role master or slave")
	}
	if _, _, err := net.SplitHostPort(bind); err != nil {
		return fmt.Errorf("cluster.bind is invalid: %w", err)
	}
	if bind == strings.TrimSpace(cfg.Server.Bind) {
		return fmt.Errorf("cluster.bind must differ from server.bind")
	}
	return nil
}

func (cfg Config) SeparateClusterListener() bool {
	return strings.TrimSpace(cfg.Cluster.Bind) != ""
}

func validateClusterMembership(cfg *Config) error {
	if cfg.Cluster.Role == "slave" {
		if strings.TrimSpace(cfg.Cluster.MasterURL) == "" {
			return fmt.Errorf("cluster.master_url is required when cluster.role is slave")
		}
		if strings.TrimSpace(cfg.Cluster.PublicURL) == "" {
			return fmt.Errorf("cluster.public_url is required when cluster.role is slave")
		}
	}
	if err := validateClusterBind(cfg); err != nil {
		return err
	}
	if cfg.Cluster.PublicURL != "" {
		if _, err := url.ParseRequestURI(cfg.Cluster.PublicURL); err != nil {
			return fmt.Errorf("cluster.public_url is invalid: %w", err)
		}
	}
	if cfg.Cluster.MasterURL != "" {
		if _, err := url.ParseRequestURI(cfg.Cluster.MasterURL); err != nil {
			return fmt.Errorf("cluster.master_url is invalid: %w", err)
		}
	}
	for _, slaveURL := range cfg.Cluster.SlaveURLs {
		if _, err := url.ParseRequestURI(slaveURL); err != nil {
			return fmt.Errorf("cluster.slave_urls contains invalid URL: %w", err)
		}
	}
	return nil
}
