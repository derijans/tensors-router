package main

import (
	"fmt"
	"strconv"
	"strings"

	"tensors-router/internal/vllm"
)

func parseWorkerConfig(arguments []string) (vllm.ClientConfig, error) {
	configuration := vllm.ClientConfig{DefaultProfile: "auto"}
	stringOptions := map[string]*string{
		"--data-dir":                   &configuration.DataDir,
		"--profile":                    &configuration.DefaultProfile,
		"--manifest":                   &configuration.ManifestPath,
		"--manifest-sha256":            &configuration.ManifestSHA256,
		"--tuf-repository-url":         &configuration.TUFRepositoryURL,
		"--tuf-root":                   &configuration.TUFRootPath,
		"--unverified-vllm-version":    &configuration.UnverifiedVLLMVersion,
		"--unverified-python-version":  &configuration.UnverifiedPythonVersion,
		"--unverified-index-url":       &configuration.UnverifiedIndexURL,
		"--unverified-extra-index-url": &configuration.UnverifiedExtraIndexURL,
	}
	boolOptions := map[string]*bool{
		"--allow-trust-remote-code":  &configuration.AllowTrustRemoteCode,
		"--allow-external-tools":     &configuration.AllowExternalTools,
		"--allow-dynamic-lora":       &configuration.AllowDynamicLoRA,
		"--oci-run-as-image-user":    &configuration.OCIRunAsImageUser,
		"--allow-unverified-install": &configuration.AllowUnverifiedInstall,
	}
	for index := 0; index < len(arguments); index += 2 {
		name := arguments[index]
		if index+1 >= len(arguments) {
			return vllm.ClientConfig{}, fmt.Errorf("%s requires a value", name)
		}
		value := strings.TrimSpace(arguments[index+1])
		if err := applyWorkerOption(&configuration, stringOptions, boolOptions, name, value); err != nil {
			return vllm.ClientConfig{}, err
		}
	}
	if err := validateWorkerConfig(configuration); err != nil {
		return vllm.ClientConfig{}, err
	}
	return configuration, nil
}

func applyWorkerOption(configuration *vllm.ClientConfig, stringOptions map[string]*string, boolOptions map[string]*bool, name string, value string) error {
	if target, found := stringOptions[name]; found {
		*target = value
		return nil
	}
	if target, found := boolOptions[name]; found {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be true or false", name)
		}
		*target = parsed
		return nil
	}
	if name != "--manifest-size" {
		return fmt.Errorf("unknown worker option %q", name)
	}
	size, err := strconv.ParseInt(value, 10, 64)
	if err != nil || size <= 0 {
		return fmt.Errorf("--manifest-size must be a positive integer")
	}
	configuration.ManifestSize = size
	return nil
}

func validateWorkerConfig(configuration vllm.ClientConfig) error {
	if configuration.DataDir == "" {
		return fmt.Errorf("worker requires --data-dir")
	}
	switch {
	case configuration.TUFRepositoryURL != "":
		if configuration.ManifestPath == "" {
			return fmt.Errorf("worker requires --manifest when --tuf-repository-url is set")
		}
	case configuration.ManifestPath != "":
		if configuration.ManifestSize <= 0 || configuration.ManifestSHA256 == "" {
			return fmt.Errorf("worker requires --manifest-size and --manifest-sha256 when --manifest is set without --tuf-repository-url")
		}
	case !configuration.AllowUnverifiedInstall:
		return fmt.Errorf("worker requires --tuf-repository-url, --manifest with --manifest-size and --manifest-sha256, or --allow-unverified-install")
	}
	return nil
}
