package config

import (
	"fmt"
	"strings"
)

func validateVLLM(cfg *Config) error {
	if strings.TrimSpace(cfg.VLLM.DataDir) == "" {
		return fmt.Errorf("vllm.data_dir is required")
	}
	if !validVLLMProfile(cfg.VLLM.Profile) {
		return fmt.Errorf("vllm.profile must be auto or a safe profile identifier")
	}
	vllmTUFConfigured := strings.TrimSpace(cfg.VLLM.TUFRepositoryURL) != ""
	vllmPinConfigured := strings.TrimSpace(cfg.VLLM.ManifestSHA256) != "" || cfg.VLLM.ManifestSize != 0
	if (vllmTUFConfigured || vllmPinConfigured) && strings.TrimSpace(cfg.VLLM.ManifestPath) == "" {
		return fmt.Errorf("vllm.manifest_path is required")
	}
	return validateVLLMManifestSource(cfg.VLLM)
}

func validVLLMProfile(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "auto" {
		return value == "auto"
	}
	if len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '.' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

// validateVLLMManifestSource accepts either a TUF-authorized manifest target or an
// operator-pinned local manifest. An empty repository URL selects the pinned mode, which
// still requires an explicit digest and length so the manifest bytes remain authorized.
func validateVLLMManifestSource(cfg VLLMConfig) error {
	if !cfg.AllowUnverifiedInstall && unverifiedInstallOptionsSet(cfg) {
		return fmt.Errorf("vllm.unverified_* options are only valid when vllm.allow_unverified_install is true")
	}
	pinConfigured := strings.TrimSpace(cfg.ManifestSHA256) != "" || cfg.ManifestSize != 0
	if strings.TrimSpace(cfg.TUFRepositoryURL) == "" {
		return validatePinnedVLLMManifest(cfg, pinConfigured)
	}
	if pinConfigured {
		return fmt.Errorf("vllm.manifest_sha256 and vllm.manifest_size are only valid when vllm.tuf_repository_url is empty")
	}
	if err := validateTUFRepositoryURL(cfg.TUFRepositoryURL); err != nil {
		return fmt.Errorf("vllm.tuf_repository_url is invalid: %w", err)
	}
	return nil
}

func unverifiedInstallOptionsSet(cfg VLLMConfig) bool {
	for _, value := range []string{cfg.UnverifiedVLLMVersion, cfg.UnverifiedPythonVersion, cfg.UnverifiedIndexURL, cfg.UnverifiedExtraIndexURL} {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func validatePinnedVLLMManifest(cfg VLLMConfig, pinConfigured bool) error {
	if !pinConfigured {
		if cfg.AllowUnverifiedInstall {
			return nil
		}
		return fmt.Errorf("vllm.manifest_sha256 and vllm.manifest_size are required when vllm.tuf_repository_url is empty and vllm.allow_unverified_install is false")
	}
	if !validSHA256Hex(cfg.ManifestSHA256) {
		return fmt.Errorf("vllm.manifest_sha256 must be a 64-character hex digest when vllm.tuf_repository_url is empty")
	}
	if cfg.ManifestSize <= 0 {
		return fmt.Errorf("vllm.manifest_size must be a positive byte count when vllm.tuf_repository_url is empty")
	}
	return nil
}
