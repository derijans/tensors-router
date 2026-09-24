package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"tensors-router/internal/vllm"
)

const (
	generateCheckInitTimeout = 4 * time.Hour
	generateCheckMaxTokens   = 16
)

type generateCheckConfig struct {
	DataDir                 string
	ModelPath               string
	ExpectedDevice          string
	Prompt                  string
	MaxTokens               int
	InstallOnly             bool
	UnverifiedVLLMVersion   string
	UnverifiedPythonVersion string
	UnverifiedIndexURL      string
	UnverifiedExtraIndexURL string
}

type generateCheckResult struct {
	Device         string `json:"device"`
	RuntimeVersion string `json:"runtime_version"`
	ManifestTrust  string `json:"manifest_trust"`
	Text           string `json:"text,omitempty"`
	InstallOnly    bool   `json:"install_only"`
}

func runGenerateCheck(arguments []string, output io.Writer) error {
	if !vllm.SupportedPlatform() {
		return fmt.Errorf("%s", vllm.UnsupportedReason())
	}
	configuration, err := parseGenerateCheckConfig(arguments)
	if err != nil {
		return err
	}
	detection, err := (vllm.SystemDetector{}).Detect(context.Background())
	if err != nil {
		return fmt.Errorf("detect generate-check hardware: %w", err)
	}
	if !containsFold(detection.Devices, configuration.ExpectedDevice) {
		return fmt.Errorf("generate-check runner did not detect required %s device", configuration.ExpectedDevice)
	}

	logs := os.Stderr
	manager, err := vllm.NewManager(vllm.ManagerOptions{
		DataDir:        configuration.DataDir,
		DefaultProfile: "auto",
		ManifestSource: vllm.UnverifiedManifestSource{
			VLLMVersion:   configuration.UnverifiedVLLMVersion,
			PythonVersion: configuration.UnverifiedPythonVersion,
		},
		Detector:   vllm.SystemDetector{},
		Downloader: vllm.HTTPArtifactDownloader{},
		Installer: vllm.UVEnvironmentInstaller{
			IndexURL:      configuration.UnverifiedIndexURL,
			ExtraIndexURL: configuration.UnverifiedExtraIndexURL,
			Logs:          logs,
			DataDir:       configuration.DataDir,
		},
		SmokeTester: vllm.CommandSmokeTester{Logs: logs},
	})
	if err != nil {
		return err
	}
	defer manager.Close()

	initContext, cancel := context.WithTimeout(context.Background(), generateCheckInitTimeout)
	defer cancel()
	if _, err := manager.StartInitialization(initContext, vllm.InitRequest{Profile: "auto"}); err != nil {
		return err
	}
	state, err := awaitGenerateCheckReady(initContext, manager)
	if err != nil {
		return err
	}

	result := generateCheckResult{
		Device:         configuration.ExpectedDevice,
		RuntimeVersion: state.RuntimeVersion,
		ManifestTrust:  state.ManifestTrust,
		InstallOnly:    configuration.InstallOnly,
	}
	if configuration.InstallOnly {
		return json.NewEncoder(output).Encode(result)
	}

	text, err := generateWithLoadedModel(initContext, manager, configuration)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("generate-check produced no text")
	}
	result.Text = text
	return json.NewEncoder(output).Encode(result)
}

func awaitGenerateCheckReady(ctx context.Context, manager *vllm.Manager) (vllm.State, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		state := manager.State(ctx)
		switch state.LifecycleState {
		case vllm.LifecycleReady:
			return state, nil
		case vllm.LifecycleFailed:
			return vllm.State{}, fmt.Errorf("generate-check installation failed: %s", state.Error)
		}
		select {
		case <-ctx.Done():
			_, _ = manager.CancelInitialization(context.Background())
			return vllm.State{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func generateWithLoadedModel(ctx context.Context, manager *vllm.Manager, configuration generateCheckConfig) (string, error) {
	digest, err := vllm.ComputeSnapshotDigest(configuration.ModelPath)
	if err != nil {
		return "", fmt.Errorf("hash generate-check model snapshot: %w", err)
	}
	modelConfig := struct {
		BackendMode string               `json:"backend_mode"`
		VLLM        vllm.VLLMModelConfig `json:"vllm"`
	}{
		BackendMode: vllm.BackendID,
		VLLM: vllm.VLLMModelConfig{
			Snapshot: vllm.SnapshotIdentity{
				Path:       configuration.ModelPath,
				TreeDigest: digest.TreeSHA256,
			},
			Runner:      "generate",
			ServedNames: []string{"generate-check"},
			Settings: vllm.CommonSettings{
				MaxModelLength: 2048,
				// On the CPU backend vLLM repurposes gpu_memory_utilization as the
				// fraction of host RAM it may reserve. vLLM's own default (~0.9) can
				// exceed what's actually free on a small or already-loaded host; this
				// model is small enough that a conservative fraction is always enough.
				GPUUtilization: 0.4,
			},
			// vLLM's default torch.compile path can take several minutes to warm up
			// on a CPU backend, which the manager's fixed runtime-startup deadline
			// does not allow for. Eager mode starts in seconds and is what this smoke
			// test actually needs to check.
			ServeArgs: []string{"--enforce-eager"},
		},
	}
	content, err := json.Marshal(modelConfig)
	if err != nil {
		return "", fmt.Errorf("encode generate-check model config: %w", err)
	}
	configPath := filepath.Join(configuration.DataDir, "generate-check.model.json")
	if err := os.WriteFile(configPath, content, 0o600); err != nil {
		return "", fmt.Errorf("write generate-check model config: %w", err)
	}

	if _, err := manager.Load(ctx, vllm.RuntimeLoadRequest{Kind: vllm.RuntimeGeneration, ConfigPath: configPath}); err != nil {
		return "", fmt.Errorf("load generate-check runtime: %w", err)
	}
	defer func() { _ = manager.Unload(context.Background(), vllm.RuntimeGeneration) }()

	backend := vllm.NewBackend(manager, vllm.RuntimeGeneration)
	requestBody, err := json.Marshal(map[string]any{
		"model":       "generate-check",
		"prompt":      configuration.Prompt,
		"max_tokens":  configuration.MaxTokens,
		"temperature": 0,
	})
	if err != nil {
		return "", fmt.Errorf("encode generate-check completion request: %w", err)
	}
	requestURL := backend.URL()
	requestURL.Path = "/v1/completions"
	httpRequest, err := newJSONRequest(ctx, requestURL.String(), requestBody)
	if err != nil {
		return "", err
	}
	response, err := backend.HTTPClient().Do(httpRequest)
	if err != nil {
		return "", fmt.Errorf("call generate-check /v1/completions: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read generate-check completion response: %w", err)
	}
	if response.StatusCode != 200 {
		return "", fmt.Errorf("generate-check /v1/completions returned HTTP %d: %s", response.StatusCode, string(responseBody))
	}
	var decoded struct {
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", fmt.Errorf("decode generate-check completion response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("generate-check completion response had no choices")
	}
	return decoded.Choices[0].Text, nil
}

func newJSONRequest(ctx context.Context, url string, body []byte) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build generate-check completion request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func parseGenerateCheckConfig(arguments []string) (generateCheckConfig, error) {
	configuration := generateCheckConfig{Prompt: "The capital of France is", MaxTokens: generateCheckMaxTokens}
	for index := 0; index < len(arguments); index++ {
		name := arguments[index]
		if index+1 >= len(arguments) {
			return generateCheckConfig{}, fmt.Errorf("%s requires a value", name)
		}
		value := strings.TrimSpace(arguments[index+1])
		index++
		switch name {
		case "--data-dir":
			configuration.DataDir = value
		case "--model":
			configuration.ModelPath = value
		case "--expected-device":
			configuration.ExpectedDevice = value
		case "--prompt":
			configuration.Prompt = value
		case "--max-tokens":
			maxTokens, err := strconv.Atoi(value)
			if err != nil || maxTokens <= 0 {
				return generateCheckConfig{}, fmt.Errorf("--max-tokens must be a positive integer")
			}
			configuration.MaxTokens = maxTokens
		case "--install-only":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return generateCheckConfig{}, fmt.Errorf("%s must be true or false", name)
			}
			configuration.InstallOnly = parsed
		case "--unverified-vllm-version":
			configuration.UnverifiedVLLMVersion = value
		case "--unverified-python-version":
			configuration.UnverifiedPythonVersion = value
		case "--unverified-index-url":
			configuration.UnverifiedIndexURL = value
		case "--unverified-extra-index-url":
			configuration.UnverifiedExtraIndexURL = value
		default:
			return generateCheckConfig{}, fmt.Errorf("unknown generate-check option %q", name)
		}
	}
	if configuration.DataDir == "" || configuration.ExpectedDevice == "" {
		return generateCheckConfig{}, fmt.Errorf("generate-check requires --data-dir and --expected-device")
	}
	if !configuration.InstallOnly && configuration.ModelPath == "" {
		return generateCheckConfig{}, fmt.Errorf("generate-check requires --model unless --install-only is true")
	}
	return configuration, nil
}
