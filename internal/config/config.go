package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"tensors-router/internal/backendendpoint"
)

type Config struct {
	Security  SecurityConfig
	Server    ServerConfig
	Auth      AuthConfig
	Models    ModelsConfig
	Backend   BackendConfig
	Kobold    KoboldConfig
	Llama     NativeServerConfig
	SDCPP     NativeServerConfig
	Logging   LoggingConfig
	Updates   UpdatesConfig
	Cluster   ClusterConfig
	Analytics AnalyticsConfig
	Limits    LimitsConfig
	Warnings  []string
}

type ServerConfig struct {
	Bind         string
	AllowedCIDRs []string
}

type AuthConfig struct {
	InferenceKeys []string
	AdminKeys     []string
	BearerKeys    []string
}

type ModelsConfig struct {
	ConfigDir    string
	StartupModel string
	FileRoots    []string
}

type BackendConfig struct {
	Mode string
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

type NativeServerConfig struct {
	BackendURL string
	BinaryPath string
	DataDir    string
	ExtraArgs  []string
	HideWindow bool
}

type LoggingConfig struct {
	Mode              string
	Enabled           bool
	BackendLogsToDisk bool
	legacyEnabledSet  bool
	modeSet           bool
}

type UpdatesConfig struct {
	Enabled             bool
	CheckInterval       time.Duration
	IncludePrereleases  bool
	BinaryURL           string
	BinarySHA256        string
	BinaryRepositoryURL string
	BinaryAssetGlob     string
	LlamaBinaryURL      string
	LlamaSHA256         string
	LlamaRepositoryURL  string
	LlamaAssetGlob      string
	SDCPPBinaryURL      string
	SDCPPSHA256         string
	SDCPPRepositoryURL  string
	SDCPPAssetGlob      string
}

type BackendUpdateSource struct {
	BinaryURL     string
	SHA256        string
	RepositoryURL string
	AssetGlob     string
}

func (updates UpdatesConfig) KoboldSource() BackendUpdateSource {
	return BackendUpdateSource{BinaryURL: updates.BinaryURL, SHA256: updates.BinarySHA256, RepositoryURL: updates.BinaryRepositoryURL, AssetGlob: updates.BinaryAssetGlob}
}

func (updates UpdatesConfig) LlamaSource() BackendUpdateSource {
	return BackendUpdateSource{BinaryURL: updates.LlamaBinaryURL, SHA256: updates.LlamaSHA256, RepositoryURL: updates.LlamaRepositoryURL, AssetGlob: updates.LlamaAssetGlob}
}

func (updates UpdatesConfig) SDCPPSource() BackendUpdateSource {
	return BackendUpdateSource{BinaryURL: updates.SDCPPBinaryURL, SHA256: updates.SDCPPSHA256, RepositoryURL: updates.SDCPPRepositoryURL, AssetGlob: updates.SDCPPAssetGlob}
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

type AnalyticsConfig struct {
	Enabled            bool
	VRAMEnabled        bool
	FlushInterval      time.Duration
	DatabasePath       string
	RawRetention       time.Duration
	VRAMSampleInterval time.Duration
}

func Defaults() Config {
	return Config{
		Security: SecurityConfig{
			Profile: SecurityProfileSecure,
		},
		Server: ServerConfig{
			Bind: "127.0.0.1:8080",
			AllowedCIDRs: []string{
				"127.0.0.0/8",
				"::1/128",
				"10.0.0.0/8",
				"172.16.0.0/12",
				"192.168.0.0/16",
			},
		},
		Auth: AuthConfig{
			InferenceKeys: []string{},
			AdminKeys:     []string{},
			BearerKeys:    []string{},
		},
		Models: ModelsConfig{
			ConfigDir: "./kcpps",
			FileRoots: []string{},
		},
		Backend: BackendConfig{
			Mode: "kobold",
		},
		Kobold: KoboldConfig{
			BackendURL:   "http://127.0.0.1:5001",
			BinaryPath:   "./bin/kobold/koboldcpp",
			DataDir:      "./data",
			Multiuser:    1,
			ExtraArgs:    []string{},
			Quiet:        true,
			SkipLauncher: true,
			NoModel:      true,
			HideWindow:   true,
		},
		Llama: NativeServerConfig{
			BackendURL: "http://127.0.0.1:5002",
			BinaryPath: "./bin/llama/llama-server",
			DataDir:    "./data/llama",
			ExtraArgs:  []string{},
			HideWindow: true,
		},
		SDCPP: NativeServerConfig{
			BackendURL: "http://127.0.0.1:7860",
			BinaryPath: "./bin/stable-diffusion/build/bin/sd-server",
			DataDir:    "./data/sdcpp",
			ExtraArgs:  []string{},
			HideWindow: true,
		},
		Logging: LoggingConfig{
			Mode:              LoggingModeNormal,
			Enabled:           true,
			BackendLogsToDisk: false,
		},
		Updates: UpdatesConfig{
			Enabled:            false,
			CheckInterval:      168 * time.Hour,
			BinaryURL:          "https://koboldai.org/cpplinuxrocm",
			BinarySHA256:       "",
			LlamaBinaryURL:     "",
			LlamaSHA256:        "",
			SDCPPBinaryURL:     "",
			SDCPPSHA256:        "",
			LlamaRepositoryURL: "https://github.com/ggml-org/llama.cpp",
			SDCPPRepositoryURL: "https://github.com/leejet/stable-diffusion.cpp",
		},
		Cluster: ClusterConfig{
			Role:           "standalone",
			NodeID:         "local",
			SlaveURLs:      []string{},
			StoreDir:       "./router-store",
			SyncInterval:   60 * time.Second,
			HealthInterval: 15 * time.Second,
		},
		Analytics: AnalyticsConfig{
			Enabled:            false,
			VRAMEnabled:        true,
			FlushInterval:      3 * time.Minute,
			RawRetention:       30 * 24 * time.Hour,
			VRAMSampleInterval: time.Second,
		},
		Limits: LimitsConfig{
			MaxControlBodyMB:    8,
			ReplayBufferMB:      64,
			MemoryBudgetMB:      2048,
			MaxStreamRequestGB:  32,
			MaxStreamResponseGB: 32,
			SelectorScanMB:      64,
			DrainTimeout:        15 * time.Minute,
		},
	}
}

func Load(path string) (Config, error) {
	return LoadWithOptions(path, LoadOptions{})
}

func LoadWithOptions(path string, options LoadOptions) (Config, error) {
	cfg := Defaults()
	if strings.TrimSpace(path) == "" {
		return cfg, fmt.Errorf("router configuration path is required")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := parseYAML(content, &cfg); err != nil {
		return cfg, err
	}
	if strings.TrimSpace(options.SecurityProfile) != "" {
		cfg.Security.Profile = strings.TrimSpace(options.SecurityProfile)
	}
	finalizeCompatibility(&cfg)

	return cfg, validate(&cfg)
}

func validate(cfg *Config) error {
	if err := validateSecurity(cfg); err != nil {
		return err
	}
	if cfg.Server.Bind == "" {
		return fmt.Errorf("server.bind is required")
	}
	if cfg.Models.ConfigDir == "" {
		return fmt.Errorf("models.config_dir is required")
	}
	for _, root := range cfg.Models.FileRoots {
		if strings.TrimSpace(root) == "" {
			return fmt.Errorf("models.file_roots cannot contain empty paths")
		}
	}
	switch cfg.Backend.Mode {
	case "kobold", "llama_sdcpp":
	default:
		return fmt.Errorf("backend.mode must be kobold or llama_sdcpp")
	}
	if _, err := backendendpoint.ParseLoopback(cfg.Kobold.BackendURL); err != nil {
		return fmt.Errorf("kobold.backend_url is invalid: %w", err)
	}
	if err := backendendpoint.RejectConflictingArgs(cfg.Kobold.ExtraArgs, "--host", "--port"); err != nil {
		return fmt.Errorf("kobold.%w", err)
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
	if cfg.Backend.Mode == "llama_sdcpp" {
		if err := validateNativeServerConfig("llama", cfg.Llama); err != nil {
			return err
		}
		if err := validateNativeServerConfig("sdcpp", cfg.SDCPP); err != nil {
			return err
		}
	}
	if cfg.Updates.CheckInterval <= 0 {
		return fmt.Errorf("updates.check_interval must be positive")
	}
	if cfg.Updates.Enabled && cfg.Backend.Mode == "kobold" {
		if err := validateUpdateSource("binary", cfg.Updates.KoboldSource()); err != nil {
			return err
		}
	}
	if cfg.Updates.Enabled && cfg.Backend.Mode == "llama_sdcpp" {
		if err := validateUpdateSource("llama", cfg.Updates.LlamaSource()); err != nil {
			return err
		}
		if err := validateUpdateSource("sdcpp", cfg.Updates.SDCPPSource()); err != nil {
			return err
		}
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
	if cfg.Analytics.FlushInterval <= 0 {
		return fmt.Errorf("analytics.flush_interval must be positive")
	}
	if cfg.Analytics.RawRetention <= 0 {
		return fmt.Errorf("analytics.raw_retention must be positive")
	}
	if cfg.Analytics.VRAMSampleInterval <= 0 {
		return fmt.Errorf("analytics.vram_sample_interval must be positive")
	}
	if err := validateLimits(cfg.Limits); err != nil {
		return err
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

func validateNativeServerConfig(section string, server NativeServerConfig) error {
	if _, err := backendendpoint.ParseLoopback(server.BackendURL); err != nil {
		return fmt.Errorf("%s.backend_url is invalid: %w", section, err)
	}
	if err := backendendpoint.RejectConflictingArgs(server.ExtraArgs, "--host", "--port", "--listen-ip", "--listen-port"); err != nil {
		return fmt.Errorf("%s.%w", section, err)
	}
	if server.BinaryPath == "" {
		return fmt.Errorf("%s.binary_path is required", section)
	}
	if server.DataDir == "" {
		return fmt.Errorf("%s.data_dir is required", section)
	}
	return nil
}

func validateUpdateSource(name string, source BackendUpdateSource) error {
	if strings.TrimSpace(source.BinaryURL) == "" && strings.TrimSpace(source.RepositoryURL) == "" {
		return fmt.Errorf("updates.%s_binary_url or updates.%s_repository_url is required when updates.enabled is true", name, name)
	}
	if err := validateHTTPSUpdateURL("updates."+name+"_binary_url", source.BinaryURL); err != nil {
		return err
	}
	if err := validateHTTPSUpdateURL("updates."+name+"_repository_url", source.RepositoryURL); err != nil {
		return err
	}
	if strings.TrimSpace(source.SHA256) != "" && !validSHA256Hex(source.SHA256) {
		return fmt.Errorf("updates.%s_binary_sha256 must be a 64 character SHA-256 hex digest when provided", name)
	}
	return nil
}

func validateHTTPSUpdateURL(field string, rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", field, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("%s must use https", field)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include a host", field)
	}
	return nil
}

func validSHA256Hex(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		switch {
		case char >= '0' && char <= '9':
		case char >= 'a' && char <= 'f':
		case char >= 'A' && char <= 'F':
		default:
			return false
		}
	}
	return true
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
	case "security":
		if key == "profile" {
			cfg.Security.Profile = value
			return nil
		}
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
	case "backend":
		switch key {
		case "mode":
			cfg.Backend.Mode = value
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
	case "llama":
		return setNativeServerScalar(&cfg.Llama, section, key, value)
	case "sdcpp":
		return setNativeServerScalar(&cfg.SDCPP, section, key, value)
	case "logging":
		switch key {
		case "mode":
			cfg.Logging.Mode = value
			cfg.Logging.modeSet = true
			return nil
		case "enabled":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Logging.Enabled = parsed
			cfg.Logging.legacyEnabledSet = true
			return nil
		case "backend_logs_to_disk":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Logging.BackendLogsToDisk = parsed
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
		case "include_prereleases":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Updates.IncludePrereleases = parsed
			return nil
		case "binary_url":
			cfg.Updates.BinaryURL = value
			return nil
		case "binary_sha256":
			cfg.Updates.BinarySHA256 = value
			return nil
		case "binary_repository_url":
			cfg.Updates.BinaryRepositoryURL = value
			return nil
		case "binary_asset_glob":
			cfg.Updates.BinaryAssetGlob = value
			return nil
		case "llama_binary_url":
			cfg.Updates.LlamaBinaryURL = value
			return nil
		case "llama_binary_sha256":
			cfg.Updates.LlamaSHA256 = value
			return nil
		case "llama_repository_url":
			cfg.Updates.LlamaRepositoryURL = value
			return nil
		case "llama_asset_glob":
			cfg.Updates.LlamaAssetGlob = value
			return nil
		case "sdcpp_binary_url":
			cfg.Updates.SDCPPBinaryURL = value
			return nil
		case "sdcpp_binary_sha256":
			cfg.Updates.SDCPPSHA256 = value
			return nil
		case "sdcpp_repository_url":
			cfg.Updates.SDCPPRepositoryURL = value
			return nil
		case "sdcpp_asset_glob":
			cfg.Updates.SDCPPAssetGlob = value
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
	case "analytics":
		switch key {
		case "enabled":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Analytics.Enabled = parsed
			return nil
		case "vram_enabled":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Analytics.VRAMEnabled = parsed
			return nil
		case "flush_interval":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.Analytics.FlushInterval = parsed
			return nil
		case "database_path":
			cfg.Analytics.DatabasePath = value
			return nil
		case "raw_retention":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.Analytics.RawRetention = parsed
			return nil
		case "vram_sample_interval":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.Analytics.VRAMSampleInterval = parsed
			return nil
		}
	case "limits":
		switch key {
		case "max_control_body_mb":
			return setPositiveInt64(&cfg.Limits.MaxControlBodyMB, value)
		case "replay_buffer_mb":
			return setPositiveInt64(&cfg.Limits.ReplayBufferMB, value)
		case "memory_budget_mb":
			return setPositiveInt64(&cfg.Limits.MemoryBudgetMB, value)
		case "max_stream_request_gb":
			return setPositiveInt64(&cfg.Limits.MaxStreamRequestGB, value)
		case "max_stream_response_gb":
			return setPositiveInt64(&cfg.Limits.MaxStreamResponseGB, value)
		case "selector_scan_mb":
			return setPositiveInt64(&cfg.Limits.SelectorScanMB, value)
		case "drain_timeout":
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return err
			}
			cfg.Limits.DrainTimeout = parsed
			return nil
		}
	}
	return fmt.Errorf("unknown key %s.%s", section, key)
}

func setPositiveInt64(target *int64, value string) error {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return err
	}
	*target = parsed
	return nil
}

func setNativeServerScalar(server *NativeServerConfig, section string, key string, value string) error {
	switch key {
	case "backend_url":
		server.BackendURL = value
		return nil
	case "binary_path":
		server.BinaryPath = value
		return nil
	case "data_dir":
		server.DataDir = value
		return nil
	case "hide_window":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		server.HideWindow = parsed
		return nil
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
		switch key {
		case "inference_keys":
			cfg.Auth.InferenceKeys = values
			return nil
		case "admin_keys":
			cfg.Auth.AdminKeys = values
			return nil
		case "bearer_keys":
			cfg.Auth.BearerKeys = values
			return nil
		}
	case "models":
		if key == "file_roots" {
			cfg.Models.FileRoots = values
			return nil
		}
	case "kobold":
		if key == "extra_args" {
			cfg.Kobold.ExtraArgs = values
			return nil
		}
	case "llama":
		if key == "extra_args" {
			cfg.Llama.ExtraArgs = values
			return nil
		}
	case "sdcpp":
		if key == "extra_args" {
			cfg.SDCPP.ExtraArgs = values
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
		switch key {
		case "inference_keys":
			cfg.Auth.InferenceKeys = append(cfg.Auth.InferenceKeys, value)
			return nil
		case "admin_keys":
			cfg.Auth.AdminKeys = append(cfg.Auth.AdminKeys, value)
			return nil
		case "bearer_keys":
			cfg.Auth.BearerKeys = append(cfg.Auth.BearerKeys, value)
			return nil
		}
	case "models":
		if key == "file_roots" {
			cfg.Models.FileRoots = append(cfg.Models.FileRoots, value)
			return nil
		}
	case "kobold":
		if key == "extra_args" {
			cfg.Kobold.ExtraArgs = append(cfg.Kobold.ExtraArgs, value)
			return nil
		}
	case "llama":
		if key == "extra_args" {
			cfg.Llama.ExtraArgs = append(cfg.Llama.ExtraArgs, value)
			return nil
		}
	case "sdcpp":
		if key == "extra_args" {
			cfg.SDCPP.ExtraArgs = append(cfg.SDCPP.ExtraArgs, value)
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
