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

func ValidateModelConfig(configuration VLLMModelConfig) error {
	if strings.TrimSpace(configuration.Snapshot.Path) == "" || !validSHA256(configuration.Snapshot.TreeDigest) {
		return fmt.Errorf("vllm.snapshot path and tree_digest are required")
	}
	if configuration.Runner != "" {
		switch configuration.Runner {
		case "generate", "pooling", "transcription":
		default:
			return fmt.Errorf("unsupported vLLM runner %q", configuration.Runner)
		}
	}
	if configuration.Task != "" && !safeIdentifier(configuration.Task) {
		return fmt.Errorf("invalid vLLM task %q", configuration.Task)
	}
	for _, servedName := range configuration.ServedNames {
		if !safeServedName(servedName) {
			return fmt.Errorf("invalid vLLM served name %q", servedName)
		}
	}
	for _, adapter := range configuration.StaticAdapters {
		if !safeAdapterName(adapter.Name) || strings.TrimSpace(adapter.Path) == "" || !validSHA256(adapter.TreeDigest) {
			return fmt.Errorf("invalid vLLM static adapter %q", adapter.Name)
		}
	}
	if configuration.Settings.MaxModelLength < 0 || configuration.Settings.TensorParallelSize < 0 || configuration.Settings.PipelineParallelSize < 0 || configuration.Settings.DataParallelSize < 0 || configuration.Settings.MaxNumberSequences < 0 {
		return fmt.Errorf("vLLM numeric settings cannot be negative")
	}
	if configuration.Settings.GPUUtilization < 0 || configuration.Settings.GPUUtilization > 1 {
		return fmt.Errorf("vLLM gpu_memory_utilization must be between 0 and 1")
	}
	for _, server := range configuration.ExternalToolServers {
		if !validExternalToolServer(server) {
			return fmt.Errorf("invalid external tool server")
		}
	}
	return ValidateServeArguments(configuration.ServeArgs)
}

func BuildServeArguments(configuration VLLMModelConfig, socketPath string, dynamicLoRA ...bool) ([]string, error) {
	if err := ValidateModelConfig(configuration); err != nil {
		return nil, err
	}
	if strings.TrimSpace(socketPath) == "" || strings.ContainsAny(socketPath, "\x00\r\n") {
		return nil, fmt.Errorf("private vLLM socket path is invalid")
	}
	arguments := []string{"-I", "-m", "vllm.entrypoints.openai.api_server", "--uds", socketPath, "--model", configuration.Snapshot.Path}
	if configuration.TrustRemoteCode {
		arguments = append(arguments, "--trust-remote-code")
	}
	if len(configuration.ExternalToolServers) > 0 {
		arguments = append(arguments, "--tool-server", strings.Join(configuration.ExternalToolServers, ","))
	}
	if configuration.Runner != "" {
		arguments = append(arguments, "--runner", configuration.Runner)
	}
	if configuration.Task != "" {
		arguments = append(arguments, "--task", configuration.Task)
	}
	if len(configuration.ServedNames) > 0 {
		arguments = append(arguments, "--served-model-name")
		arguments = append(arguments, configuration.ServedNames...)
	}
	if len(configuration.StaticAdapters) > 0 {
		arguments = append(arguments, "--lora-modules")
		for _, adapter := range configuration.StaticAdapters {
			arguments = append(arguments, adapter.Name+"="+adapter.Path)
		}
	}
	if len(configuration.StaticAdapters) > 0 || len(dynamicLoRA) > 0 && dynamicLoRA[0] {
		arguments = append(arguments, "--enable-lora")
	}
	settings := configuration.Settings
	if settings.DType != "" {
		arguments = append(arguments, "--dtype", settings.DType)
	}
	if settings.MaxModelLength > 0 {
		arguments = append(arguments, "--max-model-len", fmt.Sprint(settings.MaxModelLength))
	}
	if settings.GPUUtilization > 0 {
		arguments = append(arguments, "--gpu-memory-utilization", fmt.Sprint(settings.GPUUtilization))
	}
	if settings.TensorParallelSize > 0 {
		arguments = append(arguments, "--tensor-parallel-size", fmt.Sprint(settings.TensorParallelSize))
	}
	if settings.PipelineParallelSize > 0 {
		arguments = append(arguments, "--pipeline-parallel-size", fmt.Sprint(settings.PipelineParallelSize))
	}
	if settings.DataParallelSize > 0 {
		arguments = append(arguments, "--data-parallel-size", fmt.Sprint(settings.DataParallelSize))
	}
	if settings.MaxNumberSequences > 0 {
		arguments = append(arguments, "--max-num-seqs", fmt.Sprint(settings.MaxNumberSequences))
	}
	if settings.EnableChunkedPrefill != nil {
		arguments = append(arguments, "--enable-chunked-prefill="+fmt.Sprint(*settings.EnableChunkedPrefill))
	}
	arguments = append(arguments, "--enable-server-load-tracking")
	return append(arguments, configuration.ServeArgs...), nil
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

func ValidateServeArguments(arguments []string) error {
	forbidden := []string{
		"--host", "--port", "--uds", "--api-key", "--middleware", "--root-path", "--config", "--dev", "--ray", "--distributed-executor-backend",
		"--data-parallel-address", "--data-parallel-rpc-port", "--enable-server-load-tracking", "--enable-sleep-mode",
		"--enable-prompt-embeds", "--load-format-runai-streamer", "--grpc", "--rpc", "--kv-transfer", "--kv-events",
		"--enable-lora", "--lora-modules", "--trust-remote-code", "--enable-auto-tool-choice", "--tool-call-parser",
		"--tool-server", "--tool-parser-plugin", "--reasoning-parser-plugin", "--chat-template-content-format", "--allowed-local-media-path", "--allowed-media-domains", "--io-processor-plugin",
		"--log-config-file", "--served-model-name", "--model", "--headless", "--tokens-only",
		"--logits-processors", "--worker-cls", "--worker-extension-cls", "--hf-overrides", "--hf-token", "--hf-config-path",
		"--model-class-overrides", "--model-impl", "--tokenizer", "--tokenizer-mode", "--tokenizer-revision", "--tokenizer-pool-size", "--tokenizer-pool-type", "--tokenizer-pool-extra-config",
		"--generation-config", "--generation-config-vllm", "--chat-template", "--download-dir", "--revision", "--code-revision", "--model-loader-extra-config",
	}
	forbiddenPrefixes := []string{"--ssl-", "--ray", "--grpc", "--rpc", "--kv-transfer", "--kv-events", "--data-parallel-", "--master-", "--plugin", "--profiler"}
	for index, argument := range arguments {
		argument = strings.TrimSpace(argument)
		if argument == "" || strings.ContainsAny(argument, "\x00\r\n") {
			return fmt.Errorf("vLLM serve_args[%d] is empty or contains control characters", index)
		}
		if !strings.HasPrefix(argument, "--") {
			if strings.HasPrefix(argument, "-") || index == 0 || !strings.HasPrefix(strings.TrimSpace(arguments[index-1]), "--") {
				return fmt.Errorf("vLLM serve_args[%d] must be an option or option value", index)
			}
			continue
		}
		name := argument
		if separator := strings.IndexByte(name, '='); separator >= 0 {
			name = name[:separator]
		}
		name = strings.ReplaceAll(strings.ToLower(name), "_", "-")
		for _, blocked := range forbidden {
			if name == blocked || strings.HasPrefix(blocked, name) {
				return fmt.Errorf("vLLM serve argument %q is router-owned or out of scope", name)
			}
		}
		for _, blockedPrefix := range forbiddenPrefixes {
			if strings.HasPrefix(name, blockedPrefix) {
				return fmt.Errorf("vLLM serve argument %q is router-owned or out of scope", name)
			}
		}
	}
	return nil
}

func isolatedRuntimeEnvironment(environmentPath string, snapshotPath string) []string {
	values := map[string]string{
		"HOME": environmentPath, "HF_HUB_OFFLINE": "1", "HF_DATASETS_OFFLINE": "1", "TRANSFORMERS_OFFLINE": "1",
		"VLLM_NO_USAGE_STATS": "1", "DO_NOT_TRACK": "1", "PYTHONNOUSERSITE": "1", "PYTHONDONTWRITEBYTECODE": "1",
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
