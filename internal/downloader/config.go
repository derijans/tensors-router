package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
		Downloads: DownloadsConfig{ConcurrentJobs: 2, ConcurrentFiles: 4, RetryLimit: 5, Timeout: 30 * time.Second},
		Scanning:  ScanningConfig{HashWorkers: 1, WriteHashSidecars: true},
		Hardware:  HardwareConfig{DefaultContext: 8192, VRAMReserveMB: 1024, SafetyMarginPercent: 15},
		Logging:   LoggingConfig{Mode: "normal"},
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
	content, err := os.ReadFile(absPath)
	if err != nil {
		return Config{}, nil, err
	}
	if err := parseConfig(content, &cfg); err != nil {
		return Config{}, nil, err
	}
	if err := finalizeConfig(absPath, &cfg); err != nil {
		return Config{}, nil, err
	}
	warnings := configWarnings(absPath, cfg)
	return cfg, warnings, nil
}

func parseConfig(content []byte, cfg *Config) error {
	section := ""
	for index, rawLine := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(rawLine, "\t") {
			return fmt.Errorf("line %d: tabs are not supported", index+1)
		}
		if !strings.HasPrefix(rawLine, " ") && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			continue
		}
		if section == "" {
			return fmt.Errorf("line %d: expected a section", index+1)
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return fmt.Errorf("line %d: expected key value", index+1)
		}
		value, err := downloaderConfigString(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("line %d: %w", index+1, err)
		}
		if err := setConfigValue(cfg, section, strings.TrimSpace(key), value); err != nil {
			return fmt.Errorf("line %d: %w", index+1, err)
		}
	}
	return nil
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

func setConfigValue(cfg *Config, section string, key string, value string) error {
	parseInt64 := func() (int64, error) { return strconv.ParseInt(value, 10, 64) }
	parseNativeInt := func() (int, error) {
		parsed, err := strconv.ParseInt(value, 10, strconv.IntSize)
		if err != nil {
			return 0, err
		}
		return int(parsed), nil
	}
	parseBool := func() (bool, error) { return strconv.ParseBool(value) }
	switch section {
	case "storage":
		switch key {
		case "root":
			cfg.Storage.Root = value
		case "state_dir":
			cfg.Storage.StateDir = value
		case "database_path":
			cfg.Storage.DatabasePath = value
		case "free_space_reserve_gb":
			parsed, err := parseInt64()
			if err != nil {
				return err
			}
			cfg.Storage.FreeSpaceReserveGB = parsed
		default:
			return fmt.Errorf("unknown key %s.%s", section, key)
		}
	case "huggingface":
		if key != "token" {
			return fmt.Errorf("unknown key %s.%s", section, key)
		}
		cfg.HuggingFace.Token = value
	case "downloads":
		switch key {
		case "concurrent_jobs":
			parsed, err := parseNativeInt()
			if err != nil {
				return err
			}
			cfg.Downloads.ConcurrentJobs = parsed
		case "concurrent_files":
			parsed, err := parseNativeInt()
			if err != nil {
				return err
			}
			cfg.Downloads.ConcurrentFiles = parsed
		case "retry_limit":
			parsed, err := parseNativeInt()
			if err != nil {
				return err
			}
			cfg.Downloads.RetryLimit = parsed
		case "timeout":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.Downloads.Timeout = parsed
		default:
			return fmt.Errorf("unknown key %s.%s", section, key)
		}
	case "scanning":
		switch key {
		case "hash_workers":
			parsed, err := parseNativeInt()
			if err != nil {
				return err
			}
			cfg.Scanning.HashWorkers = parsed
		case "write_hash_sidecars":
			parsed, err := parseBool()
			if err != nil {
				return err
			}
			cfg.Scanning.WriteHashSidecars = parsed
		default:
			return fmt.Errorf("unknown key %s.%s", section, key)
		}
	case "hardware":
		switch key {
		case "default_context":
			parsed, err := parseNativeInt()
			if err != nil {
				return err
			}
			cfg.Hardware.DefaultContext = parsed
		case "vram_reserve_mb":
			parsed, err := parseInt64()
			if err != nil {
				return err
			}
			cfg.Hardware.VRAMReserveMB = parsed
		case "safety_margin_percent":
			parsed, err := parseNativeInt()
			if err != nil {
				return err
			}
			cfg.Hardware.SafetyMarginPercent = parsed
		default:
			return fmt.Errorf("unknown key %s.%s", section, key)
		}
	case "logging":
		if key != "mode" {
			return fmt.Errorf("unknown key %s.%s", section, key)
		}
		cfg.Logging.Mode = value
	default:
		return fmt.Errorf("unknown section %s", section)
	}
	return nil
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
	if cfg.Downloads.ConcurrentJobs < 1 || cfg.Downloads.ConcurrentFiles < 1 || cfg.Downloads.RetryLimit < 0 || cfg.Downloads.Timeout <= 0 {
		return fmt.Errorf("download limits are invalid")
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
	return nil
}

func pathWithin(target string, root string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func configWarnings(configPath string, cfg Config) []string {
	if strings.TrimSpace(cfg.HuggingFace.Token) == "" {
		return nil
	}
	info, err := os.Stat(configPath)
	if err != nil || info.Mode().Perm()&0o077 == 0 {
		return nil
	}
	return []string{"downloader configuration contains a token and is broadly readable"}
}
