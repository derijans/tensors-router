package webui

import (
	"os"
	"path/filepath"
	"strings"

	"tensors-router/internal/flatyaml"
)

type Config struct {
	Security SecurityConfig
	Server   ServerConfig
	Router   RouterConfig
	Logging  LoggingConfig
	Warnings []string
}

type SecurityConfig struct {
	Profile string
}

type LoggingConfig struct {
	Mode             string
	Enabled          bool
	legacyEnabledSet bool
	modeSet          bool
}

type ServerConfig struct {
	Bind               string
	BackendUIBind      string
	BackendUIPublicURL string
	StateDir           string
	CertFile           string
	KeyFile            string
	CertHosts          []string
	AdminToken         string
}

type RouterConfig struct {
	URL               string
	Token             string
	BinaryPath        string
	ConfigPath        string
	Args              []string
	StartWhenMissing  bool
	ShutdownWithWebUI bool
	SecurityProfile   string
}

type ConfigOverrides struct {
	SecurityProfile string
	Bind            string
	RouterURL       string
	RouterToken     string
	AdminToken      string
}

func DefaultConfig(executableDir string) Config {
	return Config{
		Security: SecurityConfig{Profile: SecurityProfileSecure},
		Server: ServerConfig{
			Bind:          "127.0.0.1:8443",
			BackendUIBind: "127.0.0.1:8444",
			StateDir:      filepath.Join(executableDir, "webui-state"),
		},
		Router: RouterConfig{
			URL:               "",
			BinaryPath:        filepath.Join(executableDir, routerExecutableName()),
			ConfigPath:        filepath.Join(executableDir, "config.yaml"),
			Args:              []string{},
			StartWhenMissing:  true,
			ShutdownWithWebUI: true,
		},
		Logging: LoggingConfig{
			Mode:    LoggingModeNormal,
			Enabled: true,
		},
	}
}

func LoadConfig(path string, executableDir string) (Config, error) {
	return LoadConfigWithOverrides(path, executableDir, ConfigOverrides{})
}

func LoadConfigWithOverrides(path string, executableDir string, overrides ConfigOverrides) (Config, error) {
	cfg := DefaultConfig(executableDir)
	if strings.TrimSpace(path) != "" {
		content, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return cfg, err
			}
		} else if err := flatyaml.Decode(content, webUIConfigDialect, cfg.schema()); err != nil {
			return cfg, err
		}
	}
	applyConfigOverrides(&cfg, overrides)
	if cfg.Router.ConfigPath == "" {
		cfg.Router.ConfigPath = filepath.Join(executableDir, "config.yaml")
	}
	cfg.Router.SecurityProfile = cfg.Security.Profile
	finalizeWebUICompatibility(&cfg)
	return validateWebUIConfig(cfg)
}

func routerExecutableName() string {
	if strings.EqualFold(filepath.Ext(os.Args[0]), ".exe") {
		return "tensors-router.exe"
	}
	return "tensors-router"
}
