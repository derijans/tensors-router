package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"tensors-router/internal/flatyaml"
)

func DefaultConfig(configPath string) Config {
	base := filepath.Dir(configPath)
	return Config{
		Storage: StorageConfig{
			Root:               filepath.Join(base, "models"),
			StateDir:           filepath.Join(base, "downloader-state"),
			DatabasePath:       filepath.Join(base, "downloader-state", "downloads.sqlite"),
			FreeSpaceReserveGB: 5,
		},
		Downloads: DownloadsConfig{ConcurrentJobs: 2, ConcurrentFiles: 4, RetryLimit: 8, Timeout: 30 * time.Second, StallTimeout: time.Minute},
		Scanning:  ScanningConfig{HashWorkers: 1, WriteHashSidecars: true},
		Hardware:  HardwareConfig{DefaultContext: 8192, VRAMReserveMB: 1024, SafetyMarginPercent: 15},
		Logging:   LoggingConfig{Mode: "normal", Path: filepath.Join(base, "data", "downloader.log")},
	}
}

func LoadConfig(configPath string) (Config, []string, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return Config{}, nil, fmt.Errorf("downloader config path is required")
	}
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return Config{}, nil, err
	}
	cfg := DefaultConfig(absPath)
	var warnings []string
	content, err := os.ReadFile(absPath)
	switch {
	case os.IsNotExist(err):
		warnings = append(warnings, fmt.Sprintf("downloader configuration %q does not exist; using defaults next to it", absPath))
	case err != nil:
		return Config{}, nil, err
	default:
		if err := flatyaml.Decode(content, downloaderConfigDialect, cfg.schema()); err != nil {
			return Config{}, nil, err
		}
	}
	if err := finalizeConfig(absPath, &cfg); err != nil {
		return Config{}, nil, err
	}
	warnings = append(warnings, configWarnings(absPath, cfg)...)
	if strings.TrimSpace(cfg.HuggingFace.Token) == "" {
		cfg.HuggingFace.Token = environmentHubToken()
	}
	return cfg, warnings, nil
}

func downloaderConfigString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(value, "\"") && !strings.HasPrefix(value, "'") {
		if before, _, found := strings.Cut(value, " #"); found {
			value = strings.TrimSpace(before)
		}
		return value, nil
	}
	if len(value) < 2 || value[0] != value[len(value)-1] {
		return "", fmt.Errorf("unterminated string")
	}
	if value[0] == '\'' {
		return value[1 : len(value)-1], nil
	}
	parsed, err := strconv.Unquote(value)
	if err != nil {
		return "", err
	}
	return parsed, nil
}

func finalizeConfig(configPath string, cfg *Config) error {
	base := filepath.Dir(configPath)
	resolve := func(value string) (string, error) {
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("path is required")
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(base, value)
		}
		return filepath.Abs(filepath.Clean(value))
	}
	var err error
	if cfg.Storage.Root, err = resolve(cfg.Storage.Root); err != nil {
		return err
	}
	if cfg.Storage.StateDir, err = resolve(cfg.Storage.StateDir); err != nil {
		return err
	}
	if cfg.Storage.DatabasePath, err = resolve(cfg.Storage.DatabasePath); err != nil {
		return err
	}
	if !pathWithin(cfg.Storage.DatabasePath, cfg.Storage.StateDir) {
		return fmt.Errorf("database_path must be inside state_dir")
	}
	if cfg.Storage.FreeSpaceReserveGB < 0 {
		return fmt.Errorf("free_space_reserve_gb must not be negative")
	}
	if cfg.Downloads.ConcurrentJobs < 1 || cfg.Downloads.ConcurrentFiles < 1 || cfg.Downloads.RetryLimit < 0 || cfg.Downloads.Timeout <= 0 || cfg.Downloads.StallTimeout <= 0 {
		return fmt.Errorf("download limits are invalid")
	}
	if cfg.HuggingFace.Endpoint, err = resolveHubEndpoint(cfg.HuggingFace.Endpoint); err != nil {
		return err
	}
	if cfg.Scanning.HashWorkers < 1 {
		return fmt.Errorf("hash_workers must be positive")
	}
	if cfg.Hardware.DefaultContext < 1 || cfg.Hardware.VRAMReserveMB < 0 || cfg.Hardware.SafetyMarginPercent < 0 || cfg.Hardware.SafetyMarginPercent >= 100 {
		return fmt.Errorf("hardware settings are invalid")
	}
	if cfg.Logging.Mode != "normal" && cfg.Logging.Mode != "startup_only" && cfg.Logging.Mode != "off" {
		return fmt.Errorf("logging.mode is invalid")
	}
	if cfg.Logging.Path, err = resolve(cfg.Logging.Path); err != nil {
		return err
	}
	return nil
}

func pathWithin(target string, root string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func configWarnings(configPath string, cfg Config) []string {
	if strings.TrimSpace(cfg.HuggingFace.Token) == "" || !posixPermissionsMeaningful() {
		return nil
	}
	info, err := os.Stat(configPath)
	if err != nil || info.Mode().Perm()&0o077 == 0 {
		return nil
	}
	return []string{"downloader configuration contains a token and is broadly readable"}
}

func posixPermissionsMeaningful() bool {
	return runtime.GOOS != "windows"
}
