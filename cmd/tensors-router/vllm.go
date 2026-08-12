package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"tensors-router/internal/companion"
	"tensors-router/internal/config"
	"tensors-router/internal/proxy"
	"tensors-router/internal/vllm"
)

func optionalVLLMCompanion(routerConfigPath string, configuration config.VLLMConfig, logger *log.Logger) (vllm.Service, string) {
	if !vllm.SupportedPlatform() {
		return nil, vllm.UnsupportedReason()
	}
	binaryPath, err := vllmBinaryPath(routerConfigPath, configuration.BinaryLocation)
	if err != nil {
		return nil, fmt.Sprintf("detect vLLM companion: %v", err)
	}
	info, err := os.Lstat(binaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Sprintf("vLLM companion not found at %q", binaryPath)
		}
		return nil, fmt.Sprintf("inspect vLLM companion %q: %v", binaryPath, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Sprintf("vLLM companion at %q is not a regular file", binaryPath)
	}
	configDirectory := filepath.Dir(routerConfigPath)
	client, err := vllm.StartClient(context.Background(), binaryPath, vllm.ClientConfig{
		DataDir:              resolveVLLMPath(configDirectory, configuration.DataDir),
		DefaultProfile:       configuration.Profile,
		ManifestPath:         configuration.ManifestPath,
		TUFRepositoryURL:     configuration.TUFRepositoryURL,
		TUFRootPath:          resolveOptionalVLLMPath(configDirectory, configuration.TUFRootPath),
		AllowTrustRemoteCode: configuration.TrustRemoteCode,
		AllowExternalTools:   configuration.ExternalTools,
		AllowDynamicLoRA:     configuration.DynamicLoRAEnabled,
	})
	if err != nil {
		return nil, err.Error()
	}
	logger.Printf("vLLM companion ready binary=%q", binaryPath)
	return client, ""
}

func vllmBinaryPath(routerConfigPath string, binaryLocation string) (string, error) {
	if binaryLocation != "" {
		if filepath.IsAbs(binaryLocation) {
			return binaryLocation, nil
		}
		return filepath.Join(filepath.Dir(routerConfigPath), binaryLocation), nil
	}
	executablePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if path, found := companion.FindSibling(executablePath, "tensor-router-vllm", "tensors-router"); found {
		return path, nil
	}
	return companion.PreferredSibling(executablePath, "tensor-router-vllm", "tensors-router"), nil
}

func vllmBinaryPathForState(routerConfigPath string, binaryLocation string) string {
	path, _ := vllmBinaryPath(routerConfigPath, binaryLocation)
	return path
}

func resolveVLLMPath(baseDirectory string, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(baseDirectory, value)
}

func resolveOptionalVLLMPath(baseDirectory string, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return resolveVLLMPath(baseDirectory, value)
}

func vllmBackendFamilies(service vllm.Service, configDirectory string) map[string]proxy.BackendFamilyConfig {
	if service == nil {
		return nil
	}
	generation := vllm.NewBackend(service, vllm.RuntimeGeneration, configDirectory)
	pooling := vllm.NewBackend(service, vllm.RuntimePooling, configDirectory)
	speech := vllm.NewBackend(service, vllm.RuntimeSpeech, configDirectory)
	return map[string]proxy.BackendFamilyConfig{
		proxy.BackendModeVLLM: {
			TextBackend:          generation,
			EmbeddingsBackend:    pooling,
			TranscriptionBackend: speech,
			Stop: func(ctx context.Context) error {
				return errors.Join(generation.Unload(ctx), pooling.Unload(ctx), speech.Unload(ctx))
			},
		},
	}
}

func closeVLLM(service vllm.Service) error {
	if service == nil {
		return nil
	}
	return service.Close()
}
