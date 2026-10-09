package vllm

import (
	"fmt"
	"strings"
)

func ValidateModelConfig(configuration VLLMModelConfig) error {
	if err := validateModelIdentity(configuration); err != nil {
		return err
	}
	if err := validateModelSettings(configuration.Settings); err != nil {
		return err
	}
	for _, server := range configuration.ExternalToolServers {
		if !validExternalToolServer(server) {
			return fmt.Errorf("invalid external tool server")
		}
	}
	return ValidateServeArguments(configuration.ServeArgs)
}

func validateModelIdentity(configuration VLLMModelConfig) error {
	if strings.TrimSpace(configuration.Snapshot.Path) == "" || !validSHA256(configuration.Snapshot.TreeDigest) {
		return fmt.Errorf("vllm.snapshot path and tree_digest are required")
	}
	switch configuration.Runner {
	case "", "generate", "pooling", "transcription":
	default:
		return fmt.Errorf("unsupported vLLM runner %q", configuration.Runner)
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
	return nil
}

func validateModelSettings(settings CommonSettings) error {
	for _, value := range []int{settings.MaxModelLength, settings.TensorParallelSize, settings.PipelineParallelSize, settings.DataParallelSize, settings.MaxNumberSequences} {
		if value < 0 {
			return fmt.Errorf("vLLM numeric settings cannot be negative")
		}
	}
	if settings.GPUUtilization < 0 || settings.GPUUtilization > 1 {
		return fmt.Errorf("vLLM gpu_memory_utilization must be between 0 and 1")
	}
	return nil
}

func BuildServeArguments(configuration VLLMModelConfig, socketPath string, dynamicLoRA ...bool) ([]string, error) {
	if err := ValidateModelConfig(configuration); err != nil {
		return nil, err
	}
	if strings.TrimSpace(socketPath) == "" || strings.ContainsAny(socketPath, "\x00\r\n") {
		return nil, fmt.Errorf("private vLLM socket path is invalid")
	}
	if !validServeModelPath(configuration.Snapshot.Path) {
		return nil, fmt.Errorf("vLLM snapshot path is invalid")
	}
	dynamicLoRAEnabled := len(dynamicLoRA) > 0 && dynamicLoRA[0]
	arguments := serveCommand(configuration.Snapshot.Path, socketPath)
	arguments = append(arguments, modelServeArguments(configuration, dynamicLoRAEnabled)...)
	arguments = append(arguments, settingsServeArguments(configuration.Settings)...)
	arguments = append(arguments, "--enable-server-load-tracking", "--enable-tokenizer-info-endpoint")
	return append(arguments, configuration.ServeArgs...), nil
}

func modelServeArguments(configuration VLLMModelConfig, dynamicLoRAEnabled bool) []string {
	var arguments []string
	if configuration.TrustRemoteCode {
		arguments = append(arguments, "--trust-remote-code")
	}
	if len(configuration.ExternalToolServers) > 0 {
		arguments = append(arguments, "--tool-server", strings.Join(configuration.ExternalToolServers, ","))
	}
	arguments = appendStringFlag(arguments, "--runner", configuration.Runner)
	// --task was superseded by --runner and removed outright in current vLLM, which
	// exits with "unrecognized arguments: --task" before it ever serves. Only fall back
	// to it for a legacy config that names a task without a runner.
	if configuration.Runner == "" {
		arguments = appendStringFlag(arguments, "--task", configuration.Task)
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
	if len(configuration.StaticAdapters) > 0 || dynamicLoRAEnabled {
		arguments = append(arguments, "--enable-lora")
	}
	return arguments
}

func settingsServeArguments(settings CommonSettings) []string {
	arguments := appendStringFlag(nil, "--dtype", settings.DType)
	arguments = appendPositiveFlag(arguments, "--max-model-len", settings.MaxModelLength)
	if settings.GPUUtilization > 0 {
		arguments = append(arguments, "--gpu-memory-utilization", fmt.Sprint(settings.GPUUtilization))
	}
	arguments = appendPositiveFlag(arguments, "--tensor-parallel-size", settings.TensorParallelSize)
	arguments = appendPositiveFlag(arguments, "--pipeline-parallel-size", settings.PipelineParallelSize)
	arguments = appendPositiveFlag(arguments, "--data-parallel-size", settings.DataParallelSize)
	arguments = appendPositiveFlag(arguments, "--max-num-seqs", settings.MaxNumberSequences)
	if settings.EnableChunkedPrefill != nil {
		arguments = append(arguments, "--enable-chunked-prefill="+fmt.Sprint(*settings.EnableChunkedPrefill))
	}
	return arguments
}

func appendPositiveFlag(arguments []string, flag string, value int) []string {
	if value <= 0 {
		return arguments
	}
	return append(arguments, flag, fmt.Sprint(value))
}
