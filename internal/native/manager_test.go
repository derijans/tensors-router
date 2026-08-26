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
	"tensors-router/internal/cook"
	"tensors-router/internal/portalloc"
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
		"mmproj_device":"CUDA0",
		"visionmintokens":32,
		"visionmaxtokens":512,
		"api_key_file":"C:/router/keys.txt",
		"log_prompts_dir":"C:/router/prompts",
		"reasoning_effort":"high",
		"tools_runtime":"docker:python:3.12",
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
		"--mmproj-device", "CUDA0",
		"--api-key-file", "C:/router/keys.txt",
		"--log-prompts-dir", "C:/router/prompts",
		"--reasoning-effort", "high",
		"--tools-runtime", "docker:python:3.12",
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
	if _, err := NewLlamaManager(ProcessConfig{BackendURL: "http://127.0.0.1"}); err == nil {
		t.Fatal("expected a backend URL with no port to be rejected")
	}
}

func TestDynamicPortManagersDoNotCollide(t *testing.T) {
	binaryPath := buildFakeNativeServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model.kcpps"), []byte(`{"model":"C:/models/model.gguf"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := NewLlamaManager(ProcessConfig{BackendURL: "http://127.0.0.1:0", BinaryPath: binaryPath, ConfigDir: dir, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewLlamaEmbeddingsManager(ProcessConfig{BackendURL: "http://127.0.0.1:0", BinaryPath: binaryPath, ConfigDir: dir, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.endpoint.Reserve(portalloc.Default()); err != nil {
		t.Fatal(err)
	}
	if err := second.endpoint.Reserve(portalloc.Default()); err != nil {
		t.Fatal(err)
	}
	_, firstPort := first.endpoint.HostPort()
	_, secondPort := second.endpoint.HostPort()
	if firstPort == "0" || secondPort == "0" {
		t.Fatalf("expected reserved ports, got %s and %s", firstPort, secondPort)
	}
	if firstPort == secondPort {
		t.Fatalf("expected distinct ports, both got %s", firstPort)
	}
	first.endpoint.Release(portalloc.Default())
	second.endpoint.Release(portalloc.Default())
}

func TestPinnedPortInUseFailsFast(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	_, port, err := net.SplitHostPort(held.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	binaryPath := buildFakeNativeServer(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model.kcpps"), []byte(`{"model":"C:/models/model.gguf"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	manager, err := NewLlamaManager(ProcessConfig{BackendURL: "http://127.0.0.1:" + port, BinaryPath: binaryPath, ConfigDir: dir, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	err = manager.ReloadConfig(context.Background(), "model.kcpps")
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("expected ReloadConfig to fail against an already-held port")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected a named port-in-use error, got: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("expected fast failure, took %s", elapsed)
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

func TestLegacyLlamaGPUEmbeddingAcceptsKoboldSelectors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gpu-embed.kcpps"), []byte(`{
		"backend_mode":"llama_sdcpp",
		"batchsize":512,
		"contextsize":16000,
		"embeddingsgpu":true,
		"embeddingsmodel":"/home/rocmpimp/Projects/kobold/Qwen3-Embedding-4B-Q8_0.gguf",
		"gpulayers":-1,
		"maingpu":-1,
		"model":[],
		"nomodel":true,
		"quantkv":"f16",
		"splitmode":"layer",
		"tensor_split":null,
		"threads":11,
		"usecuda":["normal","0"],
		"usemmap":true,
		"usevulkan":null
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
	args, err := manager.LaunchArguments("gpu-embed.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	for flag, value := range map[string]string{
		"--model":        "/home/rocmpimp/Projects/kobold/Qwen3-Embedding-4B-Q8_0.gguf",
		"--ctx-size":     "16000",
		"--threads":      "11",
		"--batch-size":   "512",
		"--n-gpu-layers": "-1",
		"--split-mode":   "layer",
		"--cache-type-k": "f16",
		"--cache-type-v": "f16",
	} {
		if !containsAdjacentArguments(args, flag, value) {
			t.Fatalf("missing %s %s in %#v", flag, value, args)
		}
	}
	if !containsArgument(args, "--embeddings") || containsAdjacentArguments(args, "--device", "none") {
		t.Fatalf("legacy GPU embedding launch was not preserved: %#v", args)
	}
}

func TestSeparateLlamaEmbeddingLaunchArguments(t *testing.T) {
	dir := t.TempDir()
	configs := map[string]string{
		"cpu.kcpps": `{
			"embeddingsmodel":"C:/models/cpu-embed.gguf",
			"embeddingsmaxctx":2048,
			"threads":8,
			"usecuda":["normal","0"],
			"run_embed_separate":true
		}`,
		"gpu.kcpps": `{
			"embeddingsmodel":"C:/models/gpu-embed.gguf",
			"embeddingsmaxctx":4096,
			"embeddingsgpu":true,
			"run_embed_separate":true,
			"device":"cuda",
			"splitmode":"layer",
			"tensor_split":[1,2],
			"maingpu":1
		}`,
		"global-gpu.kcpps": `{
			"embeddingsmodel":"C:/models/global-gpu-embed.gguf",
			"embeddingsgpu":true,
			"run_embed_separate":true,
			"device":"",
			"tensor_split":null,
			"maingpu":-1,
			"usecuda":["normal","0"]
		}`,
		"mixed.kcpps": `{
			"model_param":"C:/models/text.gguf",
			"embeddingsmodel":"C:/models/embed.gguf",
			"run_embed_separate":true
		}`,
	}
	for filename, content := range configs {
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	embeddingsManager, err := NewLlamaEmbeddingsManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:6005",
		BinaryPath: "llama-server",
		ConfigDir:  dir,
		DataDir:    t.TempDir(),
		ExtraArgs: []string{
			"--device", "vulkan", "--n-gpu-layers=7",
			"--split-mode", "row", "--tensor-split", "3,4", "--main-gpu", "2", "--rpc", "rpc0",
			"--parallel", "2",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cpuArgs, err := embeddingsManager.LaunchArguments("cpu.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	cpuExpected := []string{
		"--host", "127.0.0.1", "--port", "6005",
		"--model", "C:/models/cpu-embed.gguf", "--alias", "cpu", "--embeddings",
		"--ctx-size", "2048", "--threads", "8",
		"--device", "none", "--n-gpu-layers", "0", "--no-mmap", "--parallel", "2",
	}
	if !reflect.DeepEqual(cpuArgs, cpuExpected) {
		t.Fatalf("unexpected CPU embedding args %#v", cpuArgs)
	}

	gpuArgs, err := embeddingsManager.LaunchArguments("gpu.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	gpuExpected := []string{
		"--host", "127.0.0.1", "--port", "6005",
		"--model", "C:/models/gpu-embed.gguf", "--alias", "gpu", "--embeddings",
		"--ctx-size", "4096", "--n-gpu-layers", "-1", "--device", "cuda",
		"--split-mode", "layer", "--tensor-split", "1,2", "--main-gpu", "1",
		"--no-mmap", "--rpc", "rpc0", "--parallel", "2",
	}
	if !reflect.DeepEqual(gpuArgs, gpuExpected) {
		t.Fatalf("unexpected GPU embedding args %#v", gpuArgs)
	}

	globalGPUArgs, err := embeddingsManager.LaunchArguments("global-gpu.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	globalGPUExpected := []string{
		"--host", "127.0.0.1", "--port", "6005",
		"--model", "C:/models/global-gpu-embed.gguf", "--alias", "global-gpu", "--embeddings",
		"--n-gpu-layers", "-1", "--no-mmap",
		"--device", "vulkan", "--split-mode", "row", "--tensor-split", "3,4", "--main-gpu", "2", "--rpc", "rpc0",
		"--parallel", "2",
	}
	if !reflect.DeepEqual(globalGPUArgs, globalGPUExpected) {
		t.Fatalf("unexpected globally placed GPU embedding args %#v", globalGPUArgs)
	}

	primaryManager, err := NewLlamaManager(ProcessConfig{BackendURL: "http://127.0.0.1:6002", ConfigDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	primaryArgs, err := primaryManager.LaunchArguments("mixed.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if !containsAdjacentArguments(primaryArgs, "--model", "C:/models/text.gguf") || slicesContain(primaryArgs, "--embeddings") || slicesContain(primaryArgs, "C:/models/embed.gguf") {
		t.Fatalf("primary runtime retained embedding configuration %#v", primaryArgs)
	}
	if _, err := primaryManager.LaunchArguments("cpu.kcpps"); err == nil {
		t.Fatal("embedding-only config should not load in the primary runtime")
	}
}

func slicesContain(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestWhisperCPPLaunchArgumentsAndCanonicalLlamaPrecedence(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "voice.kcpps"), []byte(`{
		"whispermodel":"C:/models/whisper.bin",
		"threads":8,
		"maingpu":2,
		"flashattention":false,
		"whispercpp_processors":3,
		"whispercpp_language":"lv",
		"whispercpp_vad":true,
		"whispercpp_vad_model":"C:/models/vad.bin",
		"whispercpp_vad_threshold":0.6
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	manager, err := NewWhisperCPPManager(ProcessConfig{
		BackendURL: "http://127.0.0.1:6004",
		BinaryPath: "whisper-server",
		ConfigDir:  dir,
		DataDir:    t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	args, err := manager.LaunchArguments("voice.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"--model", "C:/models/whisper.bin", "--threads", "8", "--device", "2", "--no-flash-attn", "--processors", "3", "--language", "lv", "--vad", "--vad-model", "C:/models/vad.bin", "--vad-threshold", "0.6"} {
		if !containsArgument(args, expected) {
			t.Fatalf("missing whisper argument %q in %#v", expected, args)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "alias.kcpps"), []byte(`{"model_param":"text.gguf","parallel":1,"llama_parallel":4}`), 0o644); err != nil {
		t.Fatal(err)
	}
	llamaManager, err := NewLlamaManager(ProcessConfig{BackendURL: "http://127.0.0.1:6005", BinaryPath: "llama-server", ConfigDir: dir, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	llamaArgs, err := llamaManager.LaunchArguments("alias.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if !containsAdjacentArguments(llamaArgs, "--parallel", "4") {
		t.Fatalf("canonical llama alias did not win %#v", llamaArgs)
	}
}

func TestWhisperCPPRejectsRouterOwnedArguments(t *testing.T) {
	for _, argument := range []string{"--public", "--public-path=/ui", "--request-path", "--inference-path=/inference", "--convert", "--no-convert", "--tmp-dir=C:/tmp"} {
		_, err := NewWhisperCPPManager(ProcessConfig{BackendURL: "http://127.0.0.1:6004", BinaryPath: "whisper-server", ConfigDir: t.TempDir(), DataDir: t.TempDir(), ExtraArgs: []string{argument}})
		if err == nil {
			t.Fatalf("expected %q to be rejected", argument)
		}
	}
}

func TestWhisperCPPMapsCompleteServerOptions(t *testing.T) {
	options := map[string]any{
		"whispercpp_offset_t": 125.0, "whispercpp_offset_n": 2.0, "whispercpp_duration": 900.0,
		"whispercpp_max_context": 64.0, "whispercpp_max_len": 80.0, "whispercpp_split_on_word": true,
		"whispercpp_best_of": 3.0, "whispercpp_beam_size": 4.0, "whispercpp_audio_ctx": 768.0,
		"whispercpp_word_threshold": 0.1, "whispercpp_entropy_threshold": 2.2, "whispercpp_logprob_threshold": -0.8,
		"whispercpp_no_speech_threshold": 0.5, "whispercpp_debug": true, "whispercpp_translate": true,
		"whispercpp_diarize": true, "whispercpp_tiny_diarize": true, "whispercpp_no_fallback": true,
		"whispercpp_no_context": true, "whispercpp_detect_language": true, "whispercpp_carry_initial_prompt": true,
		"whispercpp_openvino_device": "GPU", "whispercpp_dtw": "tiny", "whispercpp_suppress_non_speech": true,
		"whispercpp_print_special": true, "whispercpp_print_colors": true, "whispercpp_print_realtime": true,
		"whispercpp_print_progress": true, "whispercpp_no_timestamps": true, "whispercpp_language_probabilities": false,
		"whispercpp_vad_min_speech_duration_ms": 250.0, "whispercpp_vad_min_silence_duration_ms": 100.0,
		"whispercpp_vad_max_speech_duration_s": 30.0, "whispercpp_vad_speech_pad_ms": 30.0,
		"whispercpp_vad_samples_overlap": 0.1,
	}
	flash := true
	args, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{WhisperModel: "whisper.bin", Threads: 8, MainGPU: 1, FlashAttention: &flash, UseCPU: true, WhisperCPPOptions: options}, "whispercpp")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"--threads", "--device", "--flash-attn", "--no-gpu", "--offset-t", "--offset-n", "--duration",
		"--max-context", "--max-len", "--split-on-word", "--best-of", "--beam-size", "--audio-ctx",
		"--word-thold", "--entropy-thold", "--logprob-thold", "--no-speech-thold", "--debug-mode",
		"--translate", "--diarize", "--tinydiarize", "--no-fallback", "--no-context", "--detect-language",
		"--carry-initial-prompt", "--ov-e-device", "--dtw", "--suppress-nst", "--print-special",
		"--print-colors", "--print-realtime", "--print-progress", "--no-timestamps", "--no-language-probabilities",
		"--vad-min-speech-duration-ms", "--vad-min-silence-duration-ms", "--vad-max-speech-duration-s",
		"--vad-speech-pad-ms", "--vad-samples-overlap",
	} {
		if !containsArgument(args, expected) {
			t.Fatalf("missing whisper argument %q in %#v", expected, args)
		}
	}
	for _, unsupported := range []string{"--output-json", "--output-json-full", "--output-srt", "--suppress-regex", "--language-probability"} {
		if containsArgument(args, unsupported) {
			t.Fatalf("unsupported v1.9.1 whisper-server argument %q in %#v", unsupported, args)
		}
	}
}

func TestLlamaRejectsRemovedTTSFlagCombinations(t *testing.T) {
	if _, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{TTSModel: "tts.gguf", TTSWAVTokenizer: "vocoder.gguf"}, "llama"); err == nil {
		t.Fatal("expected standalone ttsmodel+wavtokenizer rejection: llama.cpp removed --model-vocoder")
	}
	if _, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{ModelParam: "text.gguf", TalkerModel: "talker.gguf", Code2WAVModel: "code2wav.gguf"}, "llama"); err == nil {
		t.Fatal("expected talkermodel+code2wavmodel rejection: llama.cpp removed --model-talker and --model-vocoder")
	}
	if _, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{ModelParam: "text.gguf", TTSModel: "tts.gguf"}, "llama"); err == nil {
		t.Fatal("expected text plus standalone TTS rejection")
	}
	args, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{ModelParam: "text.gguf"}, "llama")
	if err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"--model-vocoder", "--model-talker"} {
		if containsArgument(args, removed) {
			t.Fatalf("removed llama.cpp flag %q must never be emitted, got %#v", removed, args)
		}
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
	for _, expected := range []string{"--max-vram", "cuda0=6,vulkan0=4", "--stream-layers", "--streaming", "--autofit", "--split-mode", "layer", "--circular", "--circular-x", "--circular-y"} {
		if !containsArgument(sdcppArgs, expected) {
			t.Fatalf("missing stable-diffusion.cpp argument %q in %#v", expected, sdcppArgs)
		}
	}
}

// TestSDCPPCatalogNativeFlagsAreEmitted guards against the option catalog advertising a
// sd-server flag that sdcppArguments never emits, which silently discards whatever the
// Cook UI writes for that field.
func TestSDCPPCatalogNativeFlagsAreEmitted(t *testing.T) {
	for _, definition := range cook.OptionCatalog() {
		if definition.Lane != cook.LaneImage || definition.NativeFlag == "" {
			continue
		}
		if !containsString(definition.Backends, "llama_sdcpp") {
			continue
		}
		key := definition.Key
		t.Run(key, func(t *testing.T) {
			metadata := catalog.RuntimeConfig{SDModel: "C:/models/probe.safetensors"}
			field := reflect.ValueOf(&metadata).Elem().FieldByNameFunc(func(name string) bool {
				fieldType, _ := reflect.TypeOf(metadata).FieldByName(name)
				return fieldType.Tag.Get("json") == key
			})
			if !field.IsValid() {
				t.Fatalf("catalog key %q has no matching catalog.RuntimeConfig field", key)
			}
			switch field.Kind() {
			case reflect.String:
				field.SetString("regression-test-value")
			case reflect.Int:
				field.SetInt(7)
			case reflect.Float64:
				field.SetFloat(1.5)
			case reflect.Bool:
				field.SetBool(true)
			case reflect.Interface:
				field.Set(reflect.ValueOf("regression-test-value"))
			default:
				t.Fatalf("catalog key %q maps to unsupported field kind %s", key, field.Kind())
			}
			args, err := RuntimeArgumentsForTest(metadata, "sdcpp")
			if err != nil {
				t.Fatal(err)
			}
			if !containsArgument(args, definition.NativeFlag) {
				t.Fatalf("option %q declares native flag %q but sdcppArguments never emits it: %#v", key, definition.NativeFlag, args)
			}
		})
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsArgument(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func containsAdjacentArguments(args []string, flag string, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
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
		"sdaudiovae":"C:/models/audio-vae.safetensors",
		"sdphotomaker":"C:/models/photomaker.safetensors",
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
		"sdtiledvae":768,
		"sampling_method":"euler",
		"high_noise_sampling_method":"euler_a",
		"scheduler":"karras",
		"type":"q8_0",
		"rng":"cuda",
		"sampler_rng":"cpu",
		"prediction":"v",
		"lora_apply_mode":"at_runtime",
		"cache_mode":"easycache",
		"cache_option":"0.2"
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
		"--audio-vae", "C:/models/audio-vae.safetensors",
		"--photo-maker", "C:/models/photomaker.safetensors",
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
		"--stream-layers",
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
		"--sampling-method", "euler",
		"--high-noise-sampling-method", "euler_a",
		"--scheduler", "karras",
		"--type", "q8_0",
		"--rng", "cuda",
		"--sampler-rng", "cpu",
		"--prediction", "v",
		"--lora-apply-mode", "at_runtime",
		"--cache-mode", "easycache",
		"--cache-option", "0.2",
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
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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
