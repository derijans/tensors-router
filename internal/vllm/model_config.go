package vllm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func LoadModelConfig(path string) (VLLMModelConfig, error) {
	content, err := readBoundedRegularFile(path, 4<<20)
	if err != nil {
		return VLLMModelConfig{}, err
	}
	configuration, err := ParseModelConfig(content)
	if err != nil {
		return VLLMModelConfig{}, err
	}
	configuration.Snapshot.Path = resolveModelPath(filepath.Dir(path), configuration.Snapshot.Path)
	for index := range configuration.StaticAdapters {
		configuration.StaticAdapters[index].Path = resolveModelPath(filepath.Dir(path), configuration.StaticAdapters[index].Path)
	}
	return configuration, nil
}

func ParseModelConfig(content []byte) (VLLMModelConfig, error) {
	var envelope struct {
		BackendMode string          `json:"backend_mode"`
		VLLM        json.RawMessage `json:"vllm"`
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(&envelope); err != nil {
		return VLLMModelConfig{}, fmt.Errorf("decode vLLM model config: %w", err)
	}
	if envelope.BackendMode != "" && envelope.BackendMode != BackendID {
		return VLLMModelConfig{}, fmt.Errorf("model config backend_mode must be vllm")
	}
	if len(envelope.VLLM) == 0 {
		return VLLMModelConfig{}, fmt.Errorf("model config vllm section is required")
	}
	decoder = json.NewDecoder(bytes.NewReader(envelope.VLLM))
	decoder.DisallowUnknownFields()
	var configuration VLLMModelConfig
	if err := decoder.Decode(&configuration); err != nil {
		return VLLMModelConfig{}, fmt.Errorf("decode vLLM section: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return VLLMModelConfig{}, err
	}
	if err := ValidateModelConfig(configuration); err != nil {
		return VLLMModelConfig{}, err
	}
	return configuration, nil
}

func resolveModelPath(configDirectory string, value string) string {
	value = strings.TrimSpace(value)
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(configDirectory, value)
}

func validExternalToolServer(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\x00\r\n,/@?#") {
		return false
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return false
	}
	for _, character := range port {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func isolatedRuntimeEnvironment(environmentPath string, snapshotPath string) []string {
	return isolatedRuntimeEnvironmentWithOptions(environmentPath, snapshotPath, DefaultLaunchOptions())
}

func isolatedRuntimeEnvironmentWithOptions(environmentPath string, snapshotPath string, options LaunchOptions) []string {
	values := map[string]string{
		"HOME":                 environmentPath,
		"HF_HUB_OFFLINE":       offlineFlag(options.HubOffline),
		"HF_DATASETS_OFFLINE":  offlineFlag(options.DatasetsOffline),
		"TRANSFORMERS_OFFLINE": offlineFlag(options.TransformersOffline),
		"VLLM_NO_USAGE_STATS":  "1", "DO_NOT_TRACK": "1", "PYTHONNOUSERSITE": "1", "PYTHONDONTWRITEBYTECODE": "1",
		"PIP_CONFIG_FILE": os.DevNull, "PIP_DISABLE_PIP_VERSION_CHECK": "1", "PIP_NO_INDEX": "1",
		"HF_HOME": filepath.Join(environmentPath, "hf-home"),
	}
	if snapshotPath != "" {
		values["HF_HUB_CACHE"] = filepath.Join(environmentPath, "hf-cache")
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys)+1)
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	if path := os.Getenv("PATH"); path != "" {
		environment = append(environment, "PATH="+path)
	}
	for _, key := range []string{"CUDA_VISIBLE_DEVICES", "DYLD_LIBRARY_PATH", "HIP_VISIBLE_DEVICES", "HSA_OVERRIDE_GFX_VERSION", "LD_LIBRARY_PATH", "NVIDIA_DRIVER_CAPABILITIES", "NVIDIA_VISIBLE_DEVICES", "ONEAPI_DEVICE_SELECTOR", "ROCR_VISIBLE_DEVICES", "ZE_AFFINITY_MASK"} {
		if value := os.Getenv(key); value != "" && !strings.ContainsAny(value, "\x00\r\n") {
			environment = append(environment, key+"="+value)
		}
	}
	return environment
}

func offlineFlag(enabled bool) string {
	if enabled {
		return "1"
	}
	return "0"
}
