package native

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"tensors-router/internal/catalog"
)

func TestLlamaLaunchArgumentsFromKcpps(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "combo.kcpps"), []byte(`{
		"model_param":"C:/models/text.gguf",
		"contextsize":8192,
		"threads":12,
		"blasthreads":6,
		"batchsize":512,
		"ubatchsize":256,
		"gpulayers":-1,
		"splitmode":"layer",
		"tensor_split":[1,2],
		"maingpu":1,
		"usemmap":false,
		"usemlock":true,
		"quantkv":"f16",
		"cache_type_k":"q8_0",
		"cache_type_v":"q4_0",
		"parallel":3,
		"cont_batching":false,
		"cache_ram":4096,
		"ctx_checkpoints":16,
		"kv_unified":true,
		"cache_idle_slots":false,
		"swa_full":true,
		"spec_type":"draft-simple",
		"spec_draft_type_k":"q8_0",
		"spec_draft_type_v":"q4_0",
		"mmproj":"C:/models/mmproj.gguf",
		"mmprojcpu":true,
		"visionmintokens":32,
		"visionmaxtokens":512,
		"code2wavmodel":"C:/models/code2wav.gguf",
		"api_key_file":"C:/router/keys.txt",
		"log_prompts_dir":"C:/router/prompts",
		"agent":true,
		"models_dir":"C:/models/router",
		"models_preset":"C:/models/presets.ini",
		"models_max":2,
		"models_autoload":false
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := NewLlamaManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:6002",
		BinaryPath: "llama-server",
		ConfigDir:  dir,
		DataDir:    t.TempDir(),
		ExtraArgs:  []string{"--parallel", "2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	args, err := manager.LaunchArguments("combo.kcpps")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"--host", "127.0.0.1",
		"--port", "6002",
		"--model", "C:/models/text.gguf",
		"--alias", "combo",
		"--ctx-size", "8192",
		"--threads", "12",
		"--threads-batch", "6",
		"--batch-size", "512",
		"--ubatch-size", "256",
		"--n-gpu-layers", "-1",
		"--split-mode", "layer",
		"--tensor-split", "1,2",
		"--main-gpu", "1",
		"--parallel", "3",
		"--no-cont-batching",
		"--cache-ram", "4096",
		"--ctx-checkpoints", "16",
		"--kv-unified",
		"--no-cache-idle-slots",
		"--swa-full",
		"--spec-type", "draft-simple",
		"--spec-draft-type-k", "q8_0",
		"--spec-draft-type-v", "q4_0",
		"--no-mmap",
		"--mlock",
		"--cache-type-k", "q8_0",
		"--cache-type-v", "q4_0",
		"--mmproj", "C:/models/mmproj.gguf",
		"--no-mmproj-offload",
		"--image-min-tokens", "32",
		"--image-max-tokens", "512",
		"--model-vocoder", "C:/models/code2wav.gguf",
		"--api-key-file", "C:/router/keys.txt",
		"--log-prompts-dir", "C:/router/prompts",
		"--agent",
		"--models-dir", "C:/models/router",
		"--models-preset", "C:/models/presets.ini",
		"--models-max", "2",
		"--no-models-autoload",
		"--parallel", "2",
	}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("unexpected args %#v", args)
	}
}

func TestNewManagersRejectNonLoopbackAndBindOverrides(t *testing.T) {
	if _, err := NewLlamaManager(ProcessConfig{BackendURL: "http://192.168.1.20:5002"}); err == nil {
		t.Fatal("expected non-loopback backend rejection")
	}
	if _, err := NewSDCPPManager(ProcessConfig{BackendURL: "http://127.0.0.1:7860", ExtraArgs: []string{"--listen-ip", "0.0.0.0"}}); err == nil {
		t.Fatal("expected bind override rejection")
	}
}

func TestLlamaEmbeddingLaunchArgumentsEnableEmbeddings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "embed.kcpps"), []byte(`{
		"nomodel":true,
		"embeddingsmodel":"C:/models/embed.gguf"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := NewLlamaManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:6003",
		BinaryPath: "llama-server",
		ConfigDir:  dir,
		DataDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	args, err := manager.LaunchArguments("embed.kcpps")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"--host", "127.0.0.1",
		"--port", "6003",
		"--model", "C:/models/embed.gguf",
		"--alias", "embed",
		"--no-mmap",
		"--embeddings",
	}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("unexpected args %#v", args)
	}
}

func TestCurrentReleaseArgumentsPreserveOptionalAndAssignmentValues(t *testing.T) {
	mmprojAuto := true
	llamaArgs, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{
		ModelParam:      "C:/models/text.gguf",
		MMProjAuto:      &mmprojAuto,
		SpecDraftPMin:   0.15,
		SSEPingInterval: 10,
	}, "llama")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--mmproj-auto", "--spec-draft-p-min", "0.15", "--sse-ping-interval", "10"} {
		if !containsArgument(llamaArgs, expected) {
			t.Fatalf("missing llama argument %q in %#v", expected, llamaArgs)
		}
	}

	sdcppArgs, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{
		SDModel:        "C:/models/image.safetensors",
		SDMaxVRAM:      "cuda0=6,vulkan0=4",
		SDStreamLayers: 4,
		SDStreaming:    true,
		SDAutoFit:      true,
		SDSplitMode:    "layer",
		SDCircular:     true,
		SDCircularX:    true,
		SDCircularY:    true,
	}, "sdcpp")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--max-vram", "cuda0=6,vulkan0=4", "--stream-layers", "4", "--streaming", "--autofit", "--split-mode", "layer", "--circular", "--circular-x", "--circular-y"} {
		if !containsArgument(sdcppArgs, expected) {
			t.Fatalf("missing stable-diffusion.cpp argument %q in %#v", expected, sdcppArgs)
		}
	}
}

func containsArgument(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func TestSDCPPLaunchArgumentsFromKcpps(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "image.kcpps"), []byte(`{
		"sdmodel":"C:/models/dream.safetensors",
		"sdvae":"C:/models/vae.safetensors",
		"sddiffusionmodel":"C:/models/diffusion.safetensors",
		"sdhighnoisediffusionmodel":"C:/models/high-noise.safetensors",
		"sdunconddiffusionmodel":"C:/models/uncond.safetensors",
		"sdt5xxl":"C:/models/t5.gguf",
		"sdclipl":"C:/models/clip-l.gguf",
		"sdclipg":"C:/models/clip-g.gguf",
		"sdllm":"C:/models/llm.gguf",
		"sdllmvision":"C:/models/llm-vision.gguf",
		"sdclipvision":"C:/models/clip-vision.gguf",
		"sdembeddingsconnectors":["C:/models/embed-a.gguf","C:/models/embed-b.gguf"],
		"sdcontrolnet":"C:/models/controlnet.safetensors",
		"sdpulidweights":"C:/models/pulid.safetensors",
		"sdpulididembedding":"C:/models/pulid.bin",
		"sdpulididweight":0.75,
		"sdupscaler":"C:/models/upscale.pth",
		"sdbackend":"vulkan",
		"sdparamsbackend":"cpu",
		"sdrpcservers":["127.0.0.1:9001","127.0.0.1:9002"],
		"sdmaxvram":12288,
		"sdstreamlayers":4,
		"sdtensortyperules":["vae=f16","clip=q8_0"],
		"sdvaeformat":"safetensors",
		"sdloramodeldir":"C:/models/loras",
		"sdhiresupscalersdir":"C:/models/upscalers",
		"sdthreads":8,
		"sdflashattention":true,
		"sddiffusionflashattention":true,
		"sddiffusionconvdirect":true,
		"sdvaeconvdirect":true,
		"sdoffloadcpu":true,
		"sdvaecpu":true,
		"sdtiledvae":768
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := NewSDCPPManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:7861",
		BinaryPath: "sd-server",
		ConfigDir:  dir,
		DataDir:    t.TempDir(),
		ExtraArgs:  []string{"--verbose"},
	})
	if err != nil {
		t.Fatal(err)
	}
	args, err := manager.LaunchArguments("image.kcpps")
	if err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"--listen-ip", "127.0.0.1",
		"--listen-port", "7861",
		"--model", "C:/models/dream.safetensors",
		"--vae", "C:/models/vae.safetensors",
		"--diffusion-model", "C:/models/diffusion.safetensors",
		"--high-noise-diffusion-model", "C:/models/high-noise.safetensors",
		"--uncond-diffusion-model", "C:/models/uncond.safetensors",
		"--t5xxl", "C:/models/t5.gguf",
		"--clip_l", "C:/models/clip-l.gguf",
		"--clip_g", "C:/models/clip-g.gguf",
		"--llm", "C:/models/llm.gguf",
		"--llm-vision", "C:/models/llm-vision.gguf",
		"--clip-vision", "C:/models/clip-vision.gguf",
		"--embeddings-connector", "C:/models/embed-a.gguf,C:/models/embed-b.gguf",
		"--control-net", "C:/models/controlnet.safetensors",
		"--pulid-weights", "C:/models/pulid.safetensors",
		"--pulid-id-embedding", "C:/models/pulid.bin",
		"--pulid-id-weight", "0.75",
		"--upscale-model", "C:/models/upscale.pth",
		"--backend", "vulkan",
		"--params-backend", "cpu",
		"--rpc-servers", "127.0.0.1:9001,127.0.0.1:9002",
		"--max-vram", "12288",
		"--stream-layers", "4",
		"--tensor-type-rules", "vae=f16,clip=q8_0",
		"--vae-format", "safetensors",
		"--lora-model-dir", "C:/models/loras",
		"--upscaler-model-dir", "C:/models/upscalers",
		"--threads", "8",
		"--fa",
		"--diffusion-fa",
		"--diffusion-conv-direct",
		"--vae-conv-direct",
		"--offload-to-cpu",
		"--vae-on-cpu",
		"--vae-tiling",
		"--vae-tile-size", "768x768",
		"--verbose",
	}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("unexpected args %#v", args)
	}
}

func TestNativeManagersStartAndStopServerProcessesWithoutRealModels(t *testing.T) {
	binaryPath := buildFakeNativeServer(t)
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "text.kcpps"), []byte(`{"model_param":"C:/missing/text.gguf"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "image.kcpps"), []byte(`{"sdmodel":"C:/missing/image.safetensors"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	llamaManager, err := NewLlamaManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:" + freeTCPPort(t),
		BinaryPath: binaryPath,
		ConfigDir:  configDir,
		DataDir:    filepath.Join(t.TempDir(), "llama"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := llamaManager.ReloadConfig(ctx, "text.kcpps"); err != nil {
		t.Fatal(err)
	}
	if !llamaManager.Healthy(ctx) {
		t.Fatalf("expected llama fake server to be healthy")
	}
	if err := llamaManager.Unload(ctx); err != nil {
		t.Fatal(err)
	}

	sdcppManager, err := NewSDCPPManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:" + freeTCPPort(t),
		BinaryPath: binaryPath,
		ConfigDir:  configDir,
		DataDir:    filepath.Join(t.TempDir(), "sdcpp"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sdcppManager.ReloadConfig(ctx, "image.kcpps"); err != nil {
		t.Fatal(err)
	}
	if !sdcppManager.Healthy(ctx) {
		t.Fatalf("expected sdcpp fake server to be healthy")
	}
	if err := sdcppManager.Unload(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestNativeProcessEnvPrependsBinaryDirectory(t *testing.T) {
	dir := t.TempDir()
	envName := nativeLibraryPathEnvName()
	env := nativeProcessEnv(filepath.Join(dir, "sd-server"), []string{envName + "=/existing"})
	value := testEnvValue(env, envName)
	if !strings.HasPrefix(value, dir+string(os.PathListSeparator)) {
		t.Fatalf("expected %s to start with binary dir, got %q", envName, value)
	}
	if !strings.HasSuffix(value, string(os.PathListSeparator)+"/existing") {
		t.Fatalf("expected %s to preserve existing path, got %q", envName, value)
	}
}

func buildFakeNativeServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourcePath, []byte(fakeNativeServerSource), 0o644); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(dir, "fake-native-server")
	if runtime.GOOS == "windows" {
		outputPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", outputPath, sourcePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fake native server build failed: %v\n%s", err, string(output))
	}
	return outputPath
}

func freeTCPPort(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func testEnvValue(env []string, name string) string {
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok && envNameMatches(key, name) {
			return value
		}
	}
	return ""
}

const fakeNativeServerSource = `package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	host := argValue("--host", "--listen-ip")
	if host == "" {
		host = "127.0.0.1"
	}
	port := argValue("--port", "--listen-port")
	if port == "" {
		os.Exit(2)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	})
	mux.HandleFunc("/sdapi/v1/sd-models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]any{})
	})

	server := &http.Server{
		Addr:    net.JoinHostPort(host, port),
		Handler: mux,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		os.Exit(1)
	}
}

func argValue(names ...string) string {
	for index := 1; index < len(os.Args)-1; index++ {
		for _, name := range names {
			if os.Args[index] == name {
				return os.Args[index+1]
			}
		}
	}
	return ""
}
`
