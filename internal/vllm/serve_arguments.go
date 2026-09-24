package vllm

import (
	"fmt"
	"strings"
)

const serveEntrypointModule = "vllm.entrypoints.cli.main"

var routerOwnedServeOptions = []string{
	"--host", "--port", "--uds", "--api-key", "--middleware", "--root-path", "--config", "--dev", "--ray", "--distributed-executor-backend",
	"--data-parallel-address", "--data-parallel-rpc-port", "--enable-server-load-tracking", "--enable-tokenizer-info-endpoint", "--enable-sleep-mode",
	"--enable-prompt-embeds", "--load-format-runai-streamer", "--grpc", "--rpc", "--kv-transfer", "--kv-events",
	"--enable-lora", "--lora-modules", "--trust-remote-code", "--enable-auto-tool-choice", "--tool-call-parser",
	"--tool-server", "--tool-parser-plugin", "--reasoning-parser-plugin", "--chat-template-content-format", "--allowed-local-media-path", "--allowed-media-domains", "--io-processor-plugin",
	"--log-config-file", "--served-model-name", "--model", "--headless", "--tokens-only", "--api-server-count", "--omni",
	"--logits-processors", "--worker-cls", "--worker-extension-cls", "--hf-overrides", "--hf-token", "--hf-config-path",
	"--model-class-overrides", "--model-impl", "--tokenizer", "--tokenizer-mode", "--tokenizer-revision", "--tokenizer-pool-size", "--tokenizer-pool-type", "--tokenizer-pool-extra-config",
	"--generation-config", "--generation-config-vllm", "--chat-template", "--download-dir", "--revision", "--code-revision", "--model-loader-extra-config",
}

var routerOwnedServeOptionPrefixes = []string{"--ssl-", "--ray", "--grpc", "--rpc", "--kv-transfer", "--kv-events", "--data-parallel-", "--master-", "--plugin", "--profiler"}

var completeServeOptionsSharingRouterOwnedPrefix = map[string]bool{
	"--load-format":      true,
	"--reasoning-parser": true,
}

func serveCommand(modelPath string, socketPath string) []string {
	return []string{"-I", "-m", serveEntrypointModule, "serve", modelPath, "--uds", socketPath, "--api-server-count", "1"}
}

func validServeModelPath(modelPath string) bool {
	modelPath = strings.TrimSpace(modelPath)
	return modelPath != "" && !strings.HasPrefix(modelPath, "-") && !strings.ContainsAny(modelPath, "\x00\r\n")
}

func ValidateServeArguments(arguments []string) error {
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
		if name := serveOptionName(argument); routerOwnedServeOption(name) {
			return fmt.Errorf("vLLM serve argument %q is router-owned or out of scope", name)
		}
	}
	return nil
}

func serveOptionName(argument string) string {
	name, _, _ := strings.Cut(argument, "=")
	name, _, _ = strings.Cut(name, ".")
	return strings.ReplaceAll(strings.ToLower(name), "_", "-")
}

func routerOwnedServeOption(name string) bool {
	for _, owned := range routerOwnedServeOptions {
		if name == owned || abbreviatesServeOption(name, owned) {
			return true
		}
	}
	for _, ownedPrefix := range routerOwnedServeOptionPrefixes {
		if strings.HasPrefix(name, ownedPrefix) {
			return true
		}
	}
	return false
}

func abbreviatesServeOption(name string, option string) bool {
	return strings.HasPrefix(option, name) && !completeServeOptionsSharingRouterOwnedPrefix[name]
}
