package config

import (
	"fmt"
	"net"
	"strings"
	"time"

	"tensors-router/internal/transportbody"
)

const (
	SecurityProfileSecure     = "secure"
	SecurityProfileTrustedLAN = "trusted_lan"
	LoggingModeNormal         = "normal"
	LoggingModeStartupOnly    = "startup_only"
	LoggingModeQuiet          = "quiet"
)

type SecurityConfig struct {
	Profile string
}

type LimitsConfig struct {
	MaxControlBodyMB    int64
	ReplayBufferMB      int64
	MemoryBudgetMB      int64
	MaxStreamRequestGB  int64
	MaxStreamResponseGB int64
	SelectorScanMB      int64
	DrainTimeout        time.Duration
}

type LoadOptions struct {
	SecurityProfile string
}

func ResolveSecurityProfile(cliValue string, environmentValue string) string {
	if value := strings.TrimSpace(cliValue); value != "" {
		return value
	}
	return strings.TrimSpace(environmentValue)
}

func finalizeCompatibility(cfg *Config) {
	if len(cfg.Auth.BearerKeys) > 0 {
		cfg.Auth.InferenceKeys = appendUnique(cfg.Auth.InferenceKeys, cfg.Auth.BearerKeys...)
		cfg.Warnings = append(cfg.Warnings, "auth.bearer_keys is deprecated; values are inference-only, use auth.inference_keys")
	}
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

func validateSecurity(cfg *Config) error {
	switch cfg.Security.Profile {
	case SecurityProfileSecure, SecurityProfileTrustedLAN:
	default:
		return fmt.Errorf("security.profile must be secure or trusted_lan")
	}
	switch cfg.Logging.Mode {
	case LoggingModeNormal, LoggingModeStartupOnly, LoggingModeQuiet:
	default:
		return fmt.Errorf("logging.mode must be normal, startup_only, or quiet")
	}
	if err := validateCredentialList("auth.inference_keys", cfg.Auth.InferenceKeys); err != nil {
		return err
	}
	if err := validateCredentialList("auth.admin_keys", cfg.Auth.AdminKeys); err != nil {
		return err
	}
	if err := validateCredential("cluster.token", cfg.Cluster.Token, cfg.Cluster.Role != "standalone"); err != nil {
		return err
	}
	if cfg.Security.Profile == SecurityProfileSecure && !loopbackBind(cfg.Server.Bind) {
		if !hasCredential(cfg.Auth.InferenceKeys) {
			return fmt.Errorf("auth.inference_keys is required for non-loopback secure bind")
		}
		if !hasCredential(cfg.Auth.AdminKeys) {
			return fmt.Errorf("auth.admin_keys is required for non-loopback secure bind")
		}
	}
	return nil
}

func validateLimits(limits LimitsConfig) error {
	const maxInt64 = int64(^uint64(0) >> 1)
	values := []struct {
		name  string
		value int64
		unit  int64
	}{
		{"limits.max_control_body_mb", limits.MaxControlBodyMB, 1024 * 1024},
		{"limits.replay_buffer_mb", limits.ReplayBufferMB, 1024 * 1024},
		{"limits.memory_budget_mb", limits.MemoryBudgetMB, 1024 * 1024},
		{"limits.max_stream_request_gb", limits.MaxStreamRequestGB, 1024 * 1024 * 1024},
		{"limits.max_stream_response_gb", limits.MaxStreamResponseGB, 1024 * 1024 * 1024},
		{"limits.selector_scan_mb", limits.SelectorScanMB, 1024 * 1024},
	}
	for _, value := range values {
		if value.value <= 0 {
			return fmt.Errorf("%s must be positive", value.name)
		}
		if value.value > maxInt64/value.unit {
			return fmt.Errorf("%s is too large", value.name)
		}
	}
	workingSetMB := transportbody.TransformationWorkingSet / transportbody.MiB
	minimumMemoryMB := 2*limits.ReplayBufferMB + workingSetMB
	if limits.MemoryBudgetMB < minimumMemoryMB {
		return fmt.Errorf("limits.memory_budget_mb must be at least twice limits.replay_buffer_mb plus %d MiB", workingSetMB)
	}
	maxRequestMB := limits.MaxStreamRequestGB * 1024
	if limits.ReplayBufferMB > maxRequestMB {
		return fmt.Errorf("limits.replay_buffer_mb cannot exceed limits.max_stream_request_gb")
	}
	if limits.SelectorScanMB > maxRequestMB {
		return fmt.Errorf("limits.selector_scan_mb cannot exceed limits.max_stream_request_gb")
	}
	if limits.DrainTimeout <= 0 {
		return fmt.Errorf("limits.drain_timeout must be positive")
	}
	return nil
}

func validateCredentialList(name string, values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if err := validateCredential(name, trimmed, true); err != nil {
			return err
		}
		if _, exists := seen[trimmed]; exists {
			return fmt.Errorf("%s cannot contain duplicate credentials", name)
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}

func validateCredential(name string, value string, required bool) error {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return fmt.Errorf("%s cannot be empty", name)
		}
		return nil
	}
	switch strings.ToLower(value) {
	case "change-me", "changeme", "replace-me", "replace_me":
		return fmt.Errorf("%s contains a known placeholder", name)
	}
	return nil
}

func loopbackBind(bind string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(bind))
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

func hasCredential(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	result := make([]string, 0, len(values)+len(additions))
	for _, value := range append(append([]string{}, values...), additions...) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
