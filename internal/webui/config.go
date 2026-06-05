package webui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Server ServerConfig
	Router RouterConfig
}

type ServerConfig struct {
	Bind       string
	StateDir   string
	CertFile   string
	KeyFile    string
	CertHosts  []string
	AdminToken string
}

type RouterConfig struct {
	URL               string
	Token             string
	BinaryPath        string
	ConfigPath        string
	Args              []string
	StartWhenMissing  bool
	ShutdownWithWebUI bool
}

func DefaultConfig(executableDir string) Config {
	return Config{
		Server: ServerConfig{
			Bind:     "127.0.0.1:8443",
			StateDir: filepath.Join(executableDir, "webui-state"),
		},
		Router: RouterConfig{
			URL:               "",
			BinaryPath:        filepath.Join(executableDir, routerExecutableName()),
			ConfigPath:        filepath.Join(executableDir, "config.yaml"),
			Args:              []string{},
			StartWhenMissing:  true,
			ShutdownWithWebUI: true,
		},
	}
}

func LoadConfig(path string, executableDir string) (Config, error) {
	cfg := DefaultConfig(executableDir)
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := parseWebUIConfig(content, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Server.Bind == "" {
		return cfg, fmt.Errorf("server.bind is required")
	}
	if cfg.Server.StateDir == "" {
		return cfg, fmt.Errorf("server.state_dir is required")
	}
	if cfg.Router.ConfigPath == "" {
		cfg.Router.ConfigPath = filepath.Join(executableDir, "config.yaml")
	}
	return cfg, nil
}

func routerExecutableName() string {
	if strings.EqualFold(filepath.Ext(os.Args[0]), ".exe") {
		return "tensors-router.exe"
	}
	return "tensors-router"
}

func parseWebUIConfig(content []byte, cfg *Config) error {
	section := ""
	listKey := ""
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	for index, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(rawLine, "\t") {
			return fmt.Errorf("line %d: tabs are not supported", index+1)
		}
		if !strings.HasPrefix(rawLine, " ") && strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			listKey = ""
			continue
		}
		if section == "" {
			return fmt.Errorf("line %d: expected a section", index+1)
		}
		if strings.HasPrefix(line, "- ") {
			if listKey == "" {
				return fmt.Errorf("line %d: list item without list key", index+1)
			}
			value, err := parseConfigString(strings.TrimSpace(strings.TrimPrefix(line, "- ")))
			if err != nil {
				return fmt.Errorf("line %d: %w", index+1, err)
			}
			if err := appendWebUIListValue(cfg, section, listKey, value); err != nil {
				return fmt.Errorf("line %d: %w", index+1, err)
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return fmt.Errorf("line %d: expected key value", index+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		listKey = ""
		if value == "" {
			listKey = key
			if err := setWebUIListValue(cfg, section, key, nil); err != nil {
				return fmt.Errorf("line %d: %w", index+1, err)
			}
			continue
		}
		if strings.HasPrefix(value, "[") {
			values, err := parseConfigList(value)
			if err != nil {
				return fmt.Errorf("line %d: %w", index+1, err)
			}
			if err := setWebUIListValue(cfg, section, key, values); err != nil {
				return fmt.Errorf("line %d: %w", index+1, err)
			}
			continue
		}
		scalar, err := parseConfigString(value)
		if err != nil {
			return fmt.Errorf("line %d: %w", index+1, err)
		}
		if err := setWebUIScalarValue(cfg, section, key, scalar); err != nil {
			return fmt.Errorf("line %d: %w", index+1, err)
		}
	}
	return nil
}

func setWebUIScalarValue(cfg *Config, section string, key string, value string) error {
	switch section {
	case "server":
		switch key {
		case "bind":
			cfg.Server.Bind = value
			return nil
		case "state_dir":
			cfg.Server.StateDir = value
			return nil
		case "cert_file":
			cfg.Server.CertFile = value
			return nil
		case "key_file":
			cfg.Server.KeyFile = value
			return nil
		case "admin_token":
			cfg.Server.AdminToken = value
			return nil
		}
	case "router":
		switch key {
		case "url":
			cfg.Router.URL = value
			return nil
		case "token":
			cfg.Router.Token = value
			return nil
		case "binary_path":
			cfg.Router.BinaryPath = value
			return nil
		case "config_path":
			cfg.Router.ConfigPath = value
			return nil
		case "start_when_missing":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Router.StartWhenMissing = parsed
			return nil
		case "shutdown_with_webui":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return err
			}
			cfg.Router.ShutdownWithWebUI = parsed
			return nil
		}
	}
	return fmt.Errorf("unknown key %s.%s", section, key)
}

func setWebUIListValue(cfg *Config, section string, key string, values []string) error {
	if values == nil {
		values = []string{}
	}
	if section == "server" && key == "cert_hosts" {
		cfg.Server.CertHosts = values
		return nil
	}
	if section == "router" && key == "args" {
		cfg.Router.Args = values
		return nil
	}
	return fmt.Errorf("unknown key %s.%s", section, key)
}

func appendWebUIListValue(cfg *Config, section string, key string, value string) error {
	if section == "server" && key == "cert_hosts" {
		cfg.Server.CertHosts = append(cfg.Server.CertHosts, value)
		return nil
	}
	if section == "router" && key == "args" {
		cfg.Router.Args = append(cfg.Router.Args, value)
		return nil
	}
	return fmt.Errorf("unknown key %s.%s", section, key)
}

func parseConfigString(value string) (string, error) {
	if strings.HasPrefix(value, "\"") {
		return strconv.Unquote(value)
	}
	if strings.HasPrefix(value, "'") {
		if !strings.HasSuffix(value, "'") || len(value) < 2 {
			return "", fmt.Errorf("invalid quoted string")
		}
		return strings.TrimSuffix(strings.TrimPrefix(value, "'"), "'"), nil
	}
	return value, nil
}

func parseConfigList(value string) ([]string, error) {
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
	parts := strings.Split(body, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value, err := parseConfigString(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
