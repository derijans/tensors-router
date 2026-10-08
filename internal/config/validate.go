package config

import (
	"fmt"
	"strings"

	"tensors-router/internal/backendmode"
)

func validate(cfg *Config) error {
	for _, check := range []func(*Config) error{
		validateSecurity,
		validateServer,
		validateModels,
		validateBackendMode,
		validateVLLM,
		validateKoboldEndpoints,
		validateBackendEndpointUniqueness,
		validateKoboldProcess,
		validateSplitBackendServers,
		validateUpdates,
		validateClusterRuntime,
		validateAnalytics,
		validateDiagnostics,
		func(cfg *Config) error { return validateLimits(cfg.Limits) },
		validateMCP,
		validateClusterMembership,
	} {
		if err := check(cfg); err != nil {
			return err
		}
	}
	return nil
}

func validateServer(cfg *Config) error {
	if cfg.Server.Bind == "" {
		return fmt.Errorf("server.bind is required")
	}
	return nil
}

func validateModels(cfg *Config) error {
	if cfg.Models.ConfigDir == "" {
		return fmt.Errorf("models.config_dir is required")
	}
	for _, root := range cfg.Models.FileRoots {
		if strings.TrimSpace(root) == "" {
			return fmt.Errorf("models.file_roots cannot contain empty paths")
		}
	}
	if cfg.Models.HashWorkers < 1 {
		return fmt.Errorf("models.hash_workers must be at least 1")
	}
	if cfg.Models.ConcurrentAssetTransfers < 1 {
		return fmt.Errorf("models.concurrent_asset_transfers must be at least 1")
	}
	return nil
}

func validateBackendMode(cfg *Config) error {
	if !backendmode.Valid(cfg.Backend.Mode) {
		return fmt.Errorf("backend.mode must be kobold, llama_sdcpp, or vllm")
	}
	return nil
}

func validateAnalytics(cfg *Config) error {
	if cfg.Analytics.FlushInterval <= 0 {
		return fmt.Errorf("analytics.flush_interval must be positive")
	}
	if cfg.Analytics.RawRetention <= 0 {
		return fmt.Errorf("analytics.raw_retention must be positive")
	}
	if cfg.Analytics.VRAMSampleInterval <= 0 {
		return fmt.Errorf("analytics.vram_sample_interval must be positive")
	}
	if cfg.Analytics.LoadCaptureMaxOutputMB <= 0 {
		return fmt.Errorf("analytics.load_capture_max_output_mb must be positive")
	}
	return nil
}

func validateDiagnostics(cfg *Config) error {
	if !cfg.Diagnostics.Enabled {
		return nil
	}
	if cfg.Diagnostics.Retention <= 0 {
		return fmt.Errorf("diagnostics.retention must be positive")
	}
	if cfg.Diagnostics.MaxOutputKB <= 0 {
		return fmt.Errorf("diagnostics.max_output_kb must be positive")
	}
	return nil
}

func validateMCP(cfg *Config) error {
	if strings.TrimSpace(cfg.MCP.Directory) == "" {
		return fmt.Errorf("mcp.directory is required")
	}
	return nil
}
