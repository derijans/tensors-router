package vllm

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildServeArgumentsLaunchesVLLMServeSubcommand(t *testing.T) {
	configuration := VLLMModelConfig{Snapshot: SnapshotIdentity{Path: "/models/snapshot", TreeDigest: strings.Repeat("a", 64)}}
	arguments, err := BuildServeArguments(configuration, "/private/vllm.sock")
	if err != nil {
		t.Fatal(err)
	}
	expectedPrefix := []string{"-I", "-m", "vllm.entrypoints.cli.main", "serve", "/models/snapshot", "--uds", "/private/vllm.sock", "--api-server-count", "1"}
	if len(arguments) < len(expectedPrefix) || !slices.Equal(arguments[:len(expectedPrefix)], expectedPrefix) {
		t.Fatalf("serve command prefix = %q, want %q", arguments, expectedPrefix)
	}
	if slices.Contains(arguments, "vllm.entrypoints.openai.api_server") || slices.Contains(arguments, "--model") {
		t.Fatalf("deprecated api_server launch shape remains: %q", arguments)
	}
}

func TestSmokeServerArgumentsLaunchVLLMServeSubcommand(t *testing.T) {
	arguments := smokeServerArguments("/router-smoke/vllm.sock", "/smoke-model")
	expectedPrefix := []string{"-I", "-m", "vllm.entrypoints.cli.main", "serve", "/smoke-model", "--uds", "/router-smoke/vllm.sock"}
	if !slices.Equal(arguments[:len(expectedPrefix)], expectedPrefix) {
		t.Fatalf("smoke command prefix = %q, want %q", arguments, expectedPrefix)
	}
}

func TestBuildServeArgumentsRejectsOptionShapedSnapshotPath(t *testing.T) {
	configuration := VLLMModelConfig{Snapshot: SnapshotIdentity{Path: "--config=/other/config.yaml", TreeDigest: strings.Repeat("a", 64)}}
	if arguments, err := BuildServeArguments(configuration, "/private/vllm.sock"); err == nil {
		t.Fatalf("option-shaped positional model was accepted: %q", arguments)
	}
}

func TestServeArgumentsAcceptCompleteOptionsSharingRouterOwnedPrefix(t *testing.T) {
	accepted := [][]string{
		{"--reasoning-parser", "deepseek_r1"},
		{"--reasoning_parser=qwen3"},
		{"--load-format", "safetensors"},
		{"--load-format=gguf"},
		{"--speculative-config.method", "ngram"},
	}
	for _, arguments := range accepted {
		if err := ValidateServeArguments(arguments); err != nil {
			t.Errorf("serve arguments %q rejected: %v", arguments, err)
		}
	}
}

func TestServeArgumentsRejectRouterOwnedOptionsInEveryParserForm(t *testing.T) {
	rejected := []string{
		"--trust-remote",
		"--ud=/other.sock",
		"--api-k=secret",
		"--hf-overrides.architectures=[\"Other\"]",
		"--model-loader-extra-config.path=/other",
		"--api-server-count=4",
		"--omni",
		"--grpc",
		"--reasoning-parser-plugin=bad.module",
		"--",
	}
	for _, argument := range rejected {
		if err := ValidateServeArguments([]string{argument}); err == nil {
			t.Errorf("router-owned serve argument %q accepted", argument)
		}
	}
}
