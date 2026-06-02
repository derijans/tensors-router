package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Server  ServerConfig
	Auth    AuthConfig
	Models  ModelsConfig
	Kobold  KoboldConfig
	Logging LoggingConfig
	Updates UpdatesConfig
	Cluster ClusterConfig
}

type ServerConfig struct {
	Bind         string
	AllowedCIDRs []string
}

type AuthConfig struct {
	BearerKeys []string
}

type ModelsConfig struct {
	ConfigDir    string
	StartupModel string
}

type KoboldConfig struct {
	BackendURL   string
	BinaryPath   string
	DataDir      string
	Multiuser    int
	ExtraArgs    []string
	Quiet        bool
	SkipLauncher bool
	NoModel      bool
	HideWindow   bool
}

type LoggingConfig struct {
	Enabled bool
}

type UpdatesConfig struct {
	Enabled       bool
	CheckInterval time.Duration
	BinaryURL     string
}

type ClusterConfig struct {
	Role           string
	NodeID         string
	PublicURL      string
	MasterURL      string
	SlaveURLs      []string
	Token          string
	StoreDir       string
	SyncInterval   time.Duration
	HealthInterval time.Duration
}

func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Bind: "0.0.0.0:8080",
			AllowedCIDRs: []string{
				"127.0.0.0/8",
				"::1/128",
				"10.0.0.0/8",
				"172.16.0.0/12",
				"192.168.0.0/16",
			},
		},
		Auth: AuthConfig{
			BearerKeys: []string{},
		},
		Models: ModelsConfig{
			ConfigDir: "./kcpps",
		},
		Kobold: KoboldConfig{
			BackendURL:   "http://127.0.0.1:5001",
			BinaryPath:   "./bin/koboldcpp",
			DataDir:      "./data",
			Multiuser:    1,
			ExtraArgs:    []string{},
			Quiet:        true,
			SkipLauncher: true,
			NoModel:      true,
			HideWindow:   true,
		},
		Logging: LoggingConfig{
			Enabled: true,
		},
		Updates: UpdatesConfig{
			Enabled:       true,
			CheckInterval: 168 * time.Hour,
			BinaryURL:     "https://koboldai.org/cpplinuxrocm",
		},
		Cluster: ClusterConfig{
			Role:           "standalone",
			NodeID:         "local",
			SlaveURLs:      []string{},
			StoreDir:       "./router-store",
			SyncInterval:   60 * time.Second,
			HealthInterval: 15 * time.Second,
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	if path == "" {
		return cfg, validate(cfg)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && filepath.Base(path) == "config.yaml" {
			return cfg, validate(cfg)
		}
		return cfg, err
	}

	if err := parseYAML(content, &cfg); err != nil {
		return cfg, err
	}

	return cfg, validate(cfg)
}

func validate(cfg Config) error {
	if cfg.Server.Bind == "" {
		return fmt.Errorf("server.bind is required")
	}
	if cfg.Models.ConfigDir == "" {
		return fmt.Errorf("models.config_dir is required")
	}
	if cfg.Kobold.BackendURL == "" {
		return fmt.Errorf("kobold.backend_url is required")
	}
	if _, err := url.ParseRequestURI(cfg.Kobold.BackendURL); err != nil {
		return fmt.Errorf("kobold.backend_url is invalid: %w", err)
	}
	if cfg.Kobold.BinaryPath == "" {
		return fmt.Errorf("kobold.binary_path is required")
	}
	if cfg.Kobold.DataDir == "" {
		return fmt.Errorf("kobold.data_dir is required")
	}
	if cfg.Kobold.Multiuser < 1 {
		return fmt.Errorf("kobold.multiuser must be at least 1")
	}
	if cfg.Updates.CheckInterval <= 0 {
		return fmt.Errorf("updates.check_interval must be positive")
	}
	if cfg.Updates.Enabled && cfg.Updates.BinaryURL == "" {
		return fmt.Errorf("updates.binary_url is required when updates.enabled is true")
	}
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
	if cfg.Cluster.SyncInterval <= 0 {
		return fmt.Errorf("cluster.sync_interval must be positive")
	}
	if cfg.Cluster.HealthInterval <= 0 {
		return fmt.Errorf("cluster.health_interval must be positive")
	}
	if cfg.Cluster.Role != "standalone" && strings.TrimSpace(cfg.Cluster.Token) == "" {
		return fmt.Errorf("cluster.token is required when cluster.role is not standalone")
	}
	if cfg.Cluster.Role == "slave" {
		if strings.TrimSpace(cfg.Cluster.MasterURL) == "" {
			return fmt.Errorf("cluster.master_url is required when cluster.role is slave")
		}
		if strings.TrimSpace(cfg.Cluster.PublicURL) == "" {
			return fmt.Errorf("cluster.public_url is required when cluster.role is slave")
		}
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

func parseYAML(content []byte, cfg *Config) error {
	section := ""
	listKey := ""
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")

	for lineIndex, rawLine := range lines {
		if strings.TrimSpace(rawLine) == "" || strings.HasPrefix(strings.TrimSpace(rawLine), "#") {
			continue
		}
		if strings.Contains(rawLine, "\t") {
			return fmt.Errorf("line %d: tabs are not supported", lineIndex+1)
		}

		indent := countLeadingSpaces(rawLine)
		line := strings.TrimSpace(rawLine)

		if indent == 0 && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			listKey = ""
			continue
		}

		if section == "" {
			return fmt.Errorf("line %d: expected a section", lineIndex+1)
		}

		if strings.HasPrefix(line, "- ") {
			if listKey == "" {
				return fmt.Errorf("line %d: list item without list key", lineIndex+1)
			}
			value, err := parseStringScalar(strings.TrimSpace(strings.TrimPrefix(line, "- ")))
			if err != nil {
				return fmt.Errorf("line %d: %w", lineIndex+1, err)
			}
			if err := appendListValue(cfg, section, listKey, value); err != nil {
				return fmt.Errorf("line %d: %w", lineIndex+1, err)
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return fmt.Errorf("line %d: expected key value", lineIndex+1)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		listKey = ""

		if value == "" {
			listKey = key
			if err := setListValue(cfg, section, key, nil); err != nil {
				return fmt.Errorf("line %d: %w", lineIndex+1, err)
			}
			continue
		}

		if strings.HasPrefix(value, "[") {
			values, err := parseStringList(value)
			if err != nil {
				return fmt.Errorf("line %d: %w", lineIndex+1, err)
			}
			if err := setListValue(cfg, section, key, values); err != nil {
				return fmt.Errorf("line %d: %w", lineIndex+1, err)
			}
			continue
		}

		scalar, err := parseStringScalar(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", lineIndex+1, err)
		}
		if err := setScalarValue(cfg, section, key, scalar); err != nil {
			return fmt.Errorf("line %d: %w", lineIndex+1, err)
		}
	}

	return nil
}

func countLeadingSpaces(value string) int {
	for index, char := range value {
		if char != ' ' {
			return index
		}
	}
	return len(value)
}

func parseStringScalar(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "\"") || strings.HasPrefix(value, "'") {
		if strings.HasPrefix(value, "'") {
			if !strings.HasSuffix(value, "'") || len(value) < 2 {
				return "", fmt.Errorf("invalid quoted string")
			}
			return strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'"), nil
		}
		unquoted, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		return unquoted, nil
	}
	return value, nil
}

func parseStringList(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "[]" {
		return []string{}, nil
	}
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("invalid list")
	}

	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if body == "" {
		return []string{}, nil
	}

	parts := splitListItems(body)
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value, err := parseStringScalar(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func splitListItems(value string) []string {
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false

	for _, char := range value {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote == '"' {
			current.WriteRune(char)
			escaped = true
			continue
		}
		if quote != 0 {
			current.WriteRune(char)
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			current.WriteRune(char)
			continue
		}
		if char == ',' {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(char)
	}

	parts = append(parts, current.String())
	return parts
}

func setScalarValue(cfg *Config, section string, key string, value string) error {
	switch section {
	case "server":
		if key == "bind" {
			cfg.Server.Bind = value
			return nil
		}
	case "models":
		switch key {
		case "config_dir":
			cfg.Models.ConfigDir = value
			return nil
		case "startup_model":
			cfg.Models.StartupModel = value
			return nil
		}
	case "kobold":
		switch key {
		case "backend_url":
			cfg.Kobold.BackendURL = value
			return nil
		case "binary_path":
			cfg.Kobold.BinaryPath = value
			return nil
		case "data_dir":
			cfg.Kobold.DataDir = value
			return nil
		case "multiuser":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return err
			}
			cfg.Kobold.Multiuser = parsed
			return nil
		case "quiet":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Kobold.Quiet = parsed
			return nil
		case "skip_launcher":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Kobold.SkipLauncher = parsed
			return nil
		case "no_model":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Kobold.NoModel = parsed
			return nil
		case "hide_window":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Kobold.HideWindow = parsed
			return nil
		}
	case "logging":
		switch key {
		case "enabled":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Logging.Enabled = parsed
			return nil
		}
	case "updates":
		switch key {
		case "enabled":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Updates.Enabled = parsed
			return nil
		case "check_interval":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.Updates.CheckInterval = parsed
			return nil
		case "binary_url":
			cfg.Updates.BinaryURL = value
			return nil
		}
	case "cluster":
		switch key {
		case "role":
			cfg.Cluster.Role = value
			return nil
		case "node_id":
			cfg.Cluster.NodeID = value
			return nil
		case "public_url":
			cfg.Cluster.PublicURL = value
			return nil
		case "master_url":
			cfg.Cluster.MasterURL = value
			return nil
		case "token":
			cfg.Cluster.Token = value
			return nil
		case "store_dir":
			cfg.Cluster.StoreDir = value
			return nil
		case "sync_interval":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.Cluster.SyncInterval = parsed
			return nil
		case "health_interval":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.Cluster.HealthInterval = parsed
			return nil
		}
	}
	return fmt.Errorf("unknown key %s.%s", section, key)
}

func setListValue(cfg *Config, section string, key string, values []string) error {
	if values == nil {
		values = []string{}
	}
	switch section {
	case "server":
		if key == "allowed_cidrs" {
			cfg.Server.AllowedCIDRs = values
			return nil
		}
	case "auth":
		if key == "bearer_keys" {
			cfg.Auth.BearerKeys = values
			return nil
		}
	case "kobold":
		if key == "extra_args" {
			cfg.Kobold.ExtraArgs = values
			return nil
		}
	case "cluster":
		if key == "slave_urls" {
			cfg.Cluster.SlaveURLs = values
			return nil
		}
	}
	return fmt.Errorf("unknown key %s.%s", section, key)
}

func appendListValue(cfg *Config, section string, key string, value string) error {
	switch section {
	case "server":
		if key == "allowed_cidrs" {
			cfg.Server.AllowedCIDRs = append(cfg.Server.AllowedCIDRs, value)
			return nil
		}
	case "auth":
		if key == "bearer_keys" {
			cfg.Auth.BearerKeys = append(cfg.Auth.BearerKeys, value)
			return nil
		}
	case "kobold":
		if key == "extra_args" {
			cfg.Kobold.ExtraArgs = append(cfg.Kobold.ExtraArgs, value)
			return nil
		}
	case "cluster":
		if key == "slave_urls" {
			cfg.Cluster.SlaveURLs = append(cfg.Cluster.SlaveURLs, value)
			return nil
		}
	}
	return fmt.Errorf("unknown key %s.%s", section, key)
}
