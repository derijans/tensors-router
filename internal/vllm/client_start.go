package vllm

import (
	"context"
	"fmt"
	"os/exec"

	"tensors-router/internal/processcontrol"
)

var requiredCompanionCapabilities = []string{"persistent_jobs", "atomic_environments", "generation", "pooling", "speech", "unix_socket"}

func StartClient(ctx context.Context, binaryPath string, configuration ClientConfig) (*Client, error) {
	client, err := startCompanionProcess(binaryPath, companionArguments(configuration))
	if err != nil {
		return nil, err
	}
	handshakeContext, cancel := context.WithTimeout(ctx, companionHandshakeTimeout)
	defer cancel()
	var handshake Handshake
	if err := client.call(handshakeContext, "handshake", nil, &handshake); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("vLLM companion handshake failed: %w", err)
	}
	if err := verifyCompanionHandshake(handshake); err != nil {
		_ = client.Close()
		return nil, err
	}
	client.state = handshake.State
	return client, nil
}

func startCompanionProcess(binaryPath string, arguments []string) (*Client, error) {
	command := exec.Command(binaryPath, arguments...)
	processcontrol.Prepare(command, processcontrol.Options{HideWindow: true})
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	logs := newBoundedLog(maximumRuntimeLogBytes)
	command.Stderr = logs
	if err := command.Start(); err != nil {
		return nil, err
	}
	client := &Client{command: command, input: input, pending: map[uint64]chan protocolResponse{}, done: make(chan struct{}), logs: logs}
	go client.readResponses(output)
	return client, nil
}

func verifyCompanionHandshake(handshake Handshake) error {
	if handshake.Protocol != ProtocolVersion {
		return fmt.Errorf("vLLM companion protocol %d is incompatible with required protocol %d", handshake.Protocol, ProtocolVersion)
	}
	for _, capability := range requiredCompanionCapabilities {
		if !containsString(handshake.Capabilities, capability) {
			return fmt.Errorf("vLLM companion lacks required capability %q", capability)
		}
	}
	return nil
}

func companionArguments(configuration ClientConfig) []string {
	arguments := []string{"worker", "--data-dir", configuration.DataDir, "--profile", configuration.DefaultProfile}
	arguments = appendStringFlag(arguments, "--manifest", configuration.ManifestPath)
	arguments = append(arguments, manifestTrustArguments(configuration)...)
	if configuration.AllowUnverifiedInstall {
		arguments = append(arguments, "--allow-unverified-install", "true")
		arguments = appendStringFlag(arguments, "--unverified-vllm-version", configuration.UnverifiedVLLMVersion)
		arguments = appendStringFlag(arguments, "--unverified-python-version", configuration.UnverifiedPythonVersion)
		arguments = appendStringFlag(arguments, "--unverified-index-url", configuration.UnverifiedIndexURL)
		arguments = appendStringFlag(arguments, "--unverified-extra-index-url", configuration.UnverifiedExtraIndexURL)
	}
	arguments = appendTrueFlag(arguments, "--oci-run-as-image-user", configuration.OCIRunAsImageUser)
	arguments = appendTrueFlag(arguments, "--allow-trust-remote-code", configuration.AllowTrustRemoteCode)
	arguments = appendTrueFlag(arguments, "--allow-external-tools", configuration.AllowExternalTools)
	return appendTrueFlag(arguments, "--allow-dynamic-lora", configuration.AllowDynamicLoRA)
}

func manifestTrustArguments(configuration ClientConfig) []string {
	if configuration.TUFRepositoryURL != "" {
		return appendStringFlag([]string{"--tuf-repository-url", configuration.TUFRepositoryURL}, "--tuf-root", configuration.TUFRootPath)
	}
	if configuration.ManifestSHA256 != "" || configuration.ManifestSize != 0 {
		return []string{"--manifest-size", fmt.Sprint(configuration.ManifestSize), "--manifest-sha256", configuration.ManifestSHA256}
	}
	return nil
}

func appendStringFlag(arguments []string, flag string, value string) []string {
	if value == "" {
		return arguments
	}
	return append(arguments, flag, value)
}

func appendTrueFlag(arguments []string, flag string, enabled bool) []string {
	if !enabled {
		return arguments
	}
	return append(arguments, flag, "true")
}
