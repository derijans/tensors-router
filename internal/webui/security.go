package webui

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	SecurityProfileSecure     = "secure"
	SecurityProfileTrustedLAN = "trusted_lan"
	LoggingModeNormal         = "normal"
	LoggingModeStartupOnly    = "startup_only"
	LoggingModeQuiet          = "quiet"
)

func ResolveSecurityProfile(cliValue string, environmentValue string) string {
	if value := strings.TrimSpace(cliValue); value != "" {
		return value
	}
	return strings.TrimSpace(environmentValue)
}

func applyConfigOverrides(cfg *Config, overrides ConfigOverrides) {
	if value := strings.TrimSpace(overrides.SecurityProfile); value != "" {
		cfg.Security.Profile = value
	}
	if value := strings.TrimSpace(overrides.Bind); value != "" {
		cfg.Server.Bind = value
	}
	if value := strings.TrimSpace(overrides.RouterURL); value != "" {
		cfg.Router.URL = value
	}
	if value := strings.TrimSpace(overrides.RouterToken); value != "" {
		cfg.Router.Token = value
	}
	if value := strings.TrimSpace(overrides.AdminToken); value != "" {
		cfg.Server.AdminToken = value
	}
}

func finalizeWebUICompatibility(cfg *Config) {
	if cfg.Logging.legacyEnabledSet {
		if !cfg.Logging.modeSet {
			if cfg.Logging.Enabled {
				cfg.Logging.Mode = LoggingModeNormal
			} else {
				cfg.Logging.Mode = LoggingModeQuiet
			}
		}
		cfg.Warnings = append(cfg.Warnings, "logging.enabled is deprecated; use logging.mode")
	}
	cfg.Logging.Enabled = cfg.Logging.Mode == LoggingModeNormal
}

func validateWebUIConfig(cfg Config) (Config, error) {
	if cfg.Server.Bind == "" {
		return cfg, fmt.Errorf("server.bind is required")
	}
	if _, _, err := net.SplitHostPort(NormalizeBind(cfg.Server.Bind)); err != nil {
		return cfg, fmt.Errorf("server.bind is invalid: %w", err)
	}
	if cfg.Server.BackendUIBind == "" {
		return cfg, fmt.Errorf("server.backend_ui_bind is required")
	}
	if strings.EqualFold(NormalizeBind(cfg.Server.Bind), NormalizeBind(cfg.Server.BackendUIBind)) {
		return cfg, fmt.Errorf("server.bind and server.backend_ui_bind must be different listeners")
	}
	if _, _, err := net.SplitHostPort(NormalizeBind(cfg.Server.BackendUIBind)); err != nil {
		return cfg, fmt.Errorf("server.backend_ui_bind is invalid: %w", err)
	}
	if err := validateBackendUIPublicURL(cfg.Server.BackendUIPublicURL); err != nil {
		return cfg, err
	}
	if cfg.Server.StateDir == "" {
		return cfg, fmt.Errorf("server.state_dir is required")
	}
	switch cfg.Security.Profile {
	case SecurityProfileSecure, SecurityProfileTrustedLAN:
	default:
		return cfg, fmt.Errorf("security.profile must be secure or trusted_lan")
	}
	switch cfg.Logging.Mode {
	case LoggingModeNormal, LoggingModeStartupOnly, LoggingModeQuiet:
	default:
		return cfg, fmt.Errorf("logging.mode must be normal, startup_only, or quiet")
	}
	if err := validateWebUICredential("server.admin_token", cfg.Server.AdminToken); err != nil {
		return cfg, err
	}
	if err := validateWebUICredential("router.token", cfg.Router.Token); err != nil {
		return cfg, err
	}
	if cfg.Security.Profile == SecurityProfileSecure && !webUILoopbackBind(cfg.Server.Bind) && strings.TrimSpace(cfg.Server.AdminToken) == "" {
		return cfg, fmt.Errorf("server.admin_token is required for non-loopback secure bind")
	}
	for _, argument := range cfg.Router.Args {
		argument = strings.TrimSpace(argument)
		if argument == "--" || argument == "--security-profile" || strings.HasPrefix(argument, "--security-profile=") {
			return cfg, fmt.Errorf("router.args cannot override the managed security profile")
		}
	}
	return cfg, nil
}

func validateBackendUIPublicURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("server.backend_ui_public_url must be an https origin without credentials, path, query, or fragment")
	}
	return nil
}

func validateWebUICredential(name string, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	switch strings.ToLower(value) {
	case "change-me", "changeme", "replace-me", "replace_me":
		return fmt.Errorf("%s contains a known placeholder", name)
	}
	return nil
}

func webUILoopbackBind(bind string) bool {
	host, _, err := net.SplitHostPort(NormalizeBind(bind))
	if err != nil {
		return false
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
