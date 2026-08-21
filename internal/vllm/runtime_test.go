package vllm

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnsureRouterServedNameMakesPrimaryConfigAddressable(t *testing.T) {
	servedNames := ensureRouterServedName([]string{"public-model"}, "model")
	if strings.Join(servedNames, ",") != "model,public-model" {
		t.Fatalf("router model identity was not added: %#v", servedNames)
	}
	servedNames = ensureRouterServedName([]string{"model", "public-model"}, "model")
	if strings.Join(servedNames, ",") != "model,public-model" {
		t.Fatalf("router model identity was duplicated: %#v", servedNames)
	}
}

func TestBuildServeArgumentsOwnsSecurityBoundary(t *testing.T) {
	configuration := VLLMModelConfig{
		Snapshot:    SnapshotIdentity{Path: "/models/snapshot", TreeDigest: strings.Repeat("a", 64)},
		Runner:      "generate",
		ServedNames: []string{"public-model"},
		Settings:    CommonSettings{MaxModelLength: 4096},
		ServeArgs:   []string{"--quantization", "awq", "--speculative-config={\"method\":\"ngram\"}"},
	}
	arguments, err := BuildServeArguments(configuration, "/private/vllm.sock")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, " ")
	for _, expected := range []string{"--uds /private/vllm.sock", "--model /models/snapshot", "--served-model-name public-model", "--quantization awq"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %q", expected, joined)
		}
	}
	for _, forbidden := range [][]string{{"--host", "0.0.0.0"}, {"--api-key=secret"}, {"--middleware", "bad.module"}, {"--ray-address", "host"}, {"--trust-remote-code"}} {
		configuration.ServeArgs = forbidden
		if _, err := BuildServeArguments(configuration, "/private/vllm.sock"); err == nil {
			t.Fatalf("expected forbidden argument rejection for %#v", forbidden)
		}
	}
	for _, forbidden := range []string{"--config=unsafe.yaml", "--tool_server=https://unsafe.test", "--tool-parser-plugin=bad.module", "--reasoning_parser_plugin=bad.module", "--logits-processors=bad.module", "--worker-cls=bad.module", "--worker-extension-cls=bad.module", "--hf-overrides=bad.module", "--hf-token=secret", "--hf-t=secret", "--hf-config-path=/other/config", "--model-class-overrides=bad.module", "--model-impl=bad.module", "--tokenizer=remote/repository", "--generation-config=/other/config", "--chat-template=/other/template", "--download-dir=/other/path", "--revision=untrusted", "--headless", "--tokens-only", "--master-addr=remote", "--ssl_keyfile=secret.pem", "--enable-server-load-tracking", "--enable-tokenizer-info-endpoint"} {
		configuration.ServeArgs = []string{forbidden}
		if _, err := BuildServeArguments(configuration, "/private/vllm.sock"); err == nil {
			t.Fatalf("expected forbidden argument rejection for %q", forbidden)
		}
	}
}

func TestTypedAliasesAndAdaptersCannotInjectServeOptions(t *testing.T) {
	configuration := VLLMModelConfig{Snapshot: SnapshotIdentity{Path: "/models/snapshot", TreeDigest: strings.Repeat("a", 64)}}
	for _, alias := range []string{"--model=/other/path", "alias=value", "alias value", "alias\tvalue"} {
		configuration.ServedNames = []string{alias}
		if _, err := BuildServeArguments(configuration, "/private/vllm.sock"); err == nil {
			t.Fatalf("unsafe served alias %q was accepted", alias)
		}
	}
	configuration.ServedNames = nil
	for _, adapterName := range []string{"--model", "adapter=other", "adapter name", "org/adapter"} {
		configuration.StaticAdapters = []StaticAdapter{{Name: adapterName, Path: "/models/adapter", TreeDigest: strings.Repeat("b", 64)}}
		if _, err := BuildServeArguments(configuration, "/private/vllm.sock"); err == nil {
			t.Fatalf("unsafe static adapter name %q was accepted", adapterName)
		}
	}
}

func TestBuildServeArgumentsEnablesStaticAndDynamicLoRAOnlyThroughTypedGates(t *testing.T) {
	configuration := VLLMModelConfig{
		Snapshot:       SnapshotIdentity{Path: "/models/snapshot", TreeDigest: strings.Repeat("a", 64)},
		StaticAdapters: []StaticAdapter{{Name: "adapter", Path: "/models/adapter", TreeDigest: strings.Repeat("b", 64)}},
	}
	arguments, err := BuildServeArguments(configuration, "/private/vllm.sock")
	if err != nil {
		t.Fatal(err)
	}
	if joined := strings.Join(arguments, " "); !strings.Contains(joined, "--enable-lora") || !strings.Contains(joined, "--lora-modules adapter=/models/adapter") {
		t.Fatalf("static LoRA was not safely enabled: %s", joined)
	}
	configuration.StaticAdapters = nil
	arguments, err = BuildServeArguments(configuration, "/private/vllm.sock", true)
	if err != nil || !strings.Contains(strings.Join(arguments, " "), "--enable-lora") {
		t.Fatalf("dynamic LoRA typed gate was not applied arguments=%#v error=%v", arguments, err)
	}
}

func TestBuildServeArgumentsPreservesAllAliasesAndStaticAdapters(t *testing.T) {
	configuration := VLLMModelConfig{
		Snapshot:    SnapshotIdentity{Path: "/models/snapshot", TreeDigest: strings.Repeat("a", 64)},
		ServedNames: []string{"primary", "alias"},
		StaticAdapters: []StaticAdapter{
			{Name: "first", Path: "/models/first", TreeDigest: strings.Repeat("b", 64)},
			{Name: "second", Path: "/models/second", TreeDigest: strings.Repeat("c", 64)},
		},
	}
	arguments, err := BuildServeArguments(configuration, "/private/vllm.sock")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, " ")
	if strings.Count(joined, "--served-model-name") != 1 || !strings.Contains(joined, "--served-model-name primary alias") {
		t.Fatalf("served aliases were not emitted as one list: %s", joined)
	}
	if strings.Count(joined, "--lora-modules") != 1 || !strings.Contains(joined, "--lora-modules first=/models/first second=/models/second") {
		t.Fatalf("static adapters were not emitted as one list: %s", joined)
	}
	if strings.Count(joined, "--enable-server-load-tracking") != 1 {
		t.Fatalf("router-owned load tracking was not enabled exactly once: %s", joined)
	}
	if strings.Count(joined, "--enable-tokenizer-info-endpoint") != 1 {
		t.Fatalf("router-owned tokenizer information was not enabled exactly once: %s", joined)
	}
}

func TestRuntimeHealthProbeRequiresSuccessfulHTTPHealth(t *testing.T) {
	directory, err := os.MkdirTemp("", "vrh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	socketPath := filepath.Join(directory, "health.sock")
	launcher := &smokeRuntimeLauncher{status: http.StatusServiceUnavailable}
	child, err := launcher.Start(context.Background(), "python", []string{"--uds", socketPath}, nil, directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = child.Stop(stopContext)
		cancel()
	}()
	probeContext, cancel := context.WithTimeout(context.Background(), time.Second)
	err = probeRuntimeHealth(probeContext, socketPath)
	cancel()
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Fatalf("unhealthy HTTP response was accepted: %v", err)
	}
}

func TestBuildServeArgumentsUsesTypedRemoteCodeAndToolServerGates(t *testing.T) {
	configuration := VLLMModelConfig{
		Snapshot:            SnapshotIdentity{Path: "/models/snapshot", TreeDigest: strings.Repeat("a", 64)},
		TrustRemoteCode:     true,
		ExternalToolServers: []string{"tools.internal:8443", "[2001:db8::1]:9443"},
	}
	arguments, err := BuildServeArguments(configuration, "/private/vllm.sock")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, " ")
	if !strings.Contains(joined, "--trust-remote-code") || !strings.Contains(joined, "--tool-server tools.internal:8443,[2001:db8::1]:9443") {
		t.Fatalf("typed security gates were not applied: %s", joined)
	}
	if strings.Count(joined, "--trust-remote-code") != 1 || strings.Count(joined, "--tool-server") != 1 {
		t.Fatalf("typed security gates were duplicated: %s", joined)
	}
	for _, invalid := range []string{"https://tools.internal:8443", "user@tools.internal:8443", "tools.internal", "tools.internal:http", "tools.internal:8443,evil:9443"} {
		configuration.ExternalToolServers = []string{invalid}
		if _, err := BuildServeArguments(configuration, "/private/vllm.sock"); err == nil {
			t.Fatalf("invalid tool server %q was accepted", invalid)
		}
	}
}

func TestLoadModelConfigResolvesSnapshotPathsFromConfigDirectory(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "model.kcpps")
	content := `{"backend_mode":"vllm","vllm":{"snapshot":{"path":"snapshots/model","tree_digest":"` + strings.Repeat("a", 64) + `"},"static_adapters":[{"name":"adapter","path":"adapters/a","tree_digest":"` + strings.Repeat("b", 64) + `"}]}}`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	configuration, err := LoadModelConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Snapshot.Path != filepath.Join(directory, "snapshots", "model") || configuration.StaticAdapters[0].Path != filepath.Join(directory, "adapters", "a") {
		t.Fatalf("relative paths were not resolved %#v", configuration)
	}
}

func TestRuntimeEnvironmentIsOfflineAndDoesNotForwardCredentials(t *testing.T) {
	t.Setenv("HF_TOKEN", "secret")
	t.Setenv("HUGGING_FACE_HUB_TOKEN", "secret")
	environment := isolatedRuntimeEnvironment(t.TempDir(), t.TempDir())
	joined := strings.Join(environment, "\n")
	for _, expected := range []string{"HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "PIP_NO_INDEX=1", "VLLM_NO_USAGE_STATS=1"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in environment", expected)
		}
	}
	if strings.Contains(joined, "secret") || strings.Contains(joined, "HF_TOKEN=") || strings.Contains(joined, "HUGGING_FACE_HUB_TOKEN=") {
		t.Fatalf("credential reached runtime environment: %s", joined)
	}
}

func TestOCIRuntimeUsesPrivateMountsOfflineEnvironmentAndDeviceIsolation(t *testing.T) {
	directory := t.TempDir()
	enginePath := filepath.Join(directory, "docker"+runtimeExecutableSuffix())
	if err := os.WriteFile(enginePath, []byte("engine"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	snapshotPath := filepath.Join(directory, "snapshot")
	socketDirectory := filepath.Join(directory, "socket")
	if err := os.Mkdir(snapshotPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(socketDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	configuration := VLLMModelConfig{Snapshot: SnapshotIdentity{Path: snapshotPath, TreeDigest: strings.Repeat("a", 64)}}
	active := activeEnvironment{Path: directory, InstallMethod: "oci", OCIImage: "sha256:" + strings.Repeat("b", 64), ContainerEngine: "docker", Devices: []string{"cuda"}}
	executable, arguments, environment, err := runtimeLaunchCommand(active, configuration, filepath.Join(socketDirectory, "vllm.sock"), true, DefaultLaunchOptions(), false)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, " ")
	for _, expected := range []string{"--network=none", "--read-only", "--cap-drop=ALL", "--gpus=all", "dst=/router-socket", "dst=/models/model,readonly", "HF_HUB_OFFLINE=1", "VLLM_ALLOW_RUNTIME_LORA_UPDATING=True", "--model /models/model", active.OCIImage} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("OCI runtime command missing %q: %s", expected, joined)
		}
	}
	if executable != enginePath || strings.Contains(strings.Join(environment, "\n"), "HF_TOKEN") {
		t.Fatalf("unexpected OCI launcher executable=%q environment=%q", executable, environment)
	}
}

func TestModelSecurityPolicyRequiresAdministratorOptIn(t *testing.T) {
	manager := &Manager{}
	if err := manager.validateModelSecurityPolicy(VLLMModelConfig{TrustRemoteCode: true}); err == nil {
		t.Fatal("trust_remote_code was accepted without administrator opt-in")
	}
	if err := manager.validateModelSecurityPolicy(VLLMModelConfig{ExternalToolServers: []string{"tool-server"}}); err == nil {
		t.Fatal("external tool server was accepted without administrator opt-in")
	}
	manager.options.AllowTrustRemoteCode = true
	manager.options.AllowExternalTools = true
	if err := manager.validateModelSecurityPolicy(VLLMModelConfig{TrustRemoteCode: true, ExternalToolServers: []string{"tool-server"}}); err != nil {
		t.Fatal(err)
	}
}

func TestBoundedLogKeepsTailAndRedactsSecrets(t *testing.T) {
	log := newBoundedLog(64)
	_, _ = log.Write([]byte(strings.Repeat("x", 80)))
	if len(log.String()) != 64 {
		t.Fatalf("unexpected bounded log length %d", len(log.String()))
	}
	log = newBoundedLog(256)
	_, _ = log.Write([]byte("Authorization: Bearer top-secret HF_TOKEN=hidden ordinary"))
	content := log.String()
	if strings.Contains(content, "top-secret") || strings.Contains(content, "hidden") || !strings.Contains(content, "[REDACTED]") {
		t.Fatalf("unexpected redacted log %q", content)
	}
	content = redactSensitive(`{"api_key":"json-secret"} api-key: header-secret https://user:password@example.test/path?token=query-secret&safe=yes Bearer bearer-secret`)
	for _, secret := range []string{"json-secret", "header-secret", "password", "query-secret", "bearer-secret"} {
		if strings.Contains(content, secret) {
			t.Fatalf("secret %q remained in %q", secret, content)
		}
	}
}

func TestPrepareRuntimeLoadReplacesCrashedMatchingRuntime(t *testing.T) {
	request := RuntimeLoadRequest{Kind: RuntimeGeneration, ConfigPath: "/models/model.kcpps"}
	runtimes := map[RuntimeKind]*runtimeProcess{
		RuntimeGeneration: {
			status: RuntimeStatus{Kind: RuntimeGeneration, Running: false, SocketPath: "/private/stale.sock", Error: "crashed"},
			config: request,
		},
	}
	status, reusable, staleSocketPath, err := prepareRuntimeLoad(runtimes, request)
	if err != nil || reusable || status.Running || staleSocketPath != "/private/stale.sock" {
		t.Fatalf("crashed runtime was not prepared for reload status=%#v reusable=%v socket=%q error=%v", status, reusable, staleSocketPath, err)
	}
	if runtimes[RuntimeGeneration] != nil {
		t.Fatal("crashed runtime still blocks matching load")
	}
}
