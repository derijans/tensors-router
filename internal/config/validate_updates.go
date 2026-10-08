package config

import (
	"fmt"
	"net/url"
	"strings"
)

func validateUpdates(cfg *Config) error {
	if cfg.Updates.CheckInterval <= 0 {
		return fmt.Errorf("updates.check_interval must be positive")
	}
	if !cfg.Updates.Enabled {
		return nil
	}
	for _, candidate := range enabledUpdateSources(cfg) {
		if err := validateUpdateSource(candidate.name, candidate.source); err != nil {
			return err
		}
		if updateSourceUsesTrustedRepository(candidate.source) {
			if err := validateTUFRepositoryURL(cfg.Updates.TUFRepositoryURL); err != nil {
				return err
			}
		}
	}
	return nil
}

type namedUpdateSource struct {
	name   string
	source BackendUpdateSource
}

func enabledUpdateSources(cfg *Config) []namedUpdateSource {
	switch cfg.Backend.Mode {
	case "kobold":
		return []namedUpdateSource{{name: "binary", source: cfg.Updates.KoboldSource()}}
	case "llama_sdcpp":
		return []namedUpdateSource{
			{name: "llama", source: cfg.Updates.LlamaSource()},
			{name: "sdcpp", source: cfg.Updates.SDCPPSource()},
			{name: "whispercpp", source: cfg.Updates.WhisperCPPSource()},
		}
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
	if strings.TrimSpace(source.BinaryURL) != "" && strings.TrimSpace(source.SHA256) == "" {
		return fmt.Errorf("updates.%s_binary_sha256 is required for a direct binary URL", name)
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

func updateSourceUsesTrustedRepository(source BackendUpdateSource) bool {
	return strings.TrimSpace(source.BinaryURL) == "" && strings.TrimSpace(source.RepositoryURL) != ""
}

func validateTUFRepositoryURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return fmt.Errorf("updates.tuf_repository_url is required for repository updates")
	}
	if err := validateHTTPSUpdateURL("updates.tuf_repository_url", rawURL); err != nil {
		return err
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("updates.tuf_repository_url is invalid: %w", err)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("updates.tuf_repository_url cannot contain credentials, a query, or a fragment")
	}
	if !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/metadata") {
		return fmt.Errorf("updates.tuf_repository_url must end with /metadata")
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
