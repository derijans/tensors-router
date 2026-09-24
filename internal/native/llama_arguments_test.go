package native

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
)

var llamaLoadFlagsRemovedUpstream = []string{"--mmap", "--no-mmap", "--mlock", "--direct-io"}

func TestLlamaLoadModeReplacesRemovedMemoryMappingFlags(t *testing.T) {
	cases := []struct {
		name     string
		metadata catalog.RuntimeConfig
		want     string
	}{
		{"legacy neither", catalog.RuntimeConfig{}, "none"},
		{"legacy mmap", catalog.RuntimeConfig{UseMMap: true}, "mmap"},
		{"legacy mlock", catalog.RuntimeConfig{UseMLock: true}, "mlock"},
		{"legacy mmap and mlock", catalog.RuntimeConfig{UseMMap: true, UseMLock: true}, "mmap+mlock"},
		{"explicit auto", catalog.RuntimeConfig{LoadMode: "auto"}, "auto"},
		{"explicit dio", catalog.RuntimeConfig{LoadMode: "dio"}, "dio"},
		{"explicit wins over legacy", catalog.RuntimeConfig{LoadMode: "none", UseMMap: true, UseMLock: true}, "none"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.metadata.ModelParam = "C:/models/text.gguf"
			args, err := RuntimeArgumentsForTest(testCase.metadata, "llama")
			if err != nil {
				t.Fatal(err)
			}
			if !containsAdjacentArguments(args, "--load-mode", testCase.want) {
				t.Fatalf("expected --load-mode %s in %#v", testCase.want, args)
			}
			for _, removed := range llamaLoadFlagsRemovedUpstream {
				if containsArgument(args, removed) {
					t.Fatalf("llama-server b11157 removed %s, got %#v", removed, args)
				}
			}
		})
	}
}

func TestLlamaRejectsUnknownLoadMode(t *testing.T) {
	_, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{ModelParam: "C:/models/text.gguf", LoadMode: "mmap,mlock"}, "llama")
	if err == nil || !strings.Contains(err.Error(), "load_mode") {
		t.Fatalf("expected an unknown load_mode to fail launch instead of being dropped, got %v", err)
	}
	_, err = llamaEmbeddingArguments(catalog.RuntimeConfig{EmbeddingsModel: "C:/models/embed.gguf", RunEmbedSeparate: true, LoadMode: "fast"}, launchTarget{})
	if err == nil {
		t.Fatal("expected the embeddings runtime to reject an unknown load_mode too")
	}
}

func TestLlamaFlashAttentionTakesOnOffValue(t *testing.T) {
	disabled := false
	args, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{ModelParam: "C:/models/text.gguf", FlashAttention: &disabled, Parallel: 2}, "llama")
	if err != nil {
		t.Fatal(err)
	}
	if !containsAdjacentArguments(args, "--flash-attn", "off") || containsArgument(args, "--no-flash-attn") {
		t.Fatalf("llama-server b11157 declares -fa/--flash-attn [on|off|auto] and has no --no-flash-attn, got %#v", args)
	}
}

func TestLlamaReasoningPreserveIsABooleanFlagPair(t *testing.T) {
	cases := map[string]string{
		`true`:    "--reasoning-preserve",
		`false`:   "--no-reasoning-preserve",
		`"true"`:  "--reasoning-preserve",
		`"false"`: "--no-reasoning-preserve",
	}
	for raw, want := range cases {
		metadata, err := catalog.DecodeRuntimeConfig([]byte(`{"model_param":"C:/models/text.gguf","reasoning_preserve":` + raw + `}`))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		args, err := RuntimeArgumentsForTest(metadata, "llama")
		if err != nil {
			t.Fatal(err)
		}
		index := indexOfArgument(args, want)
		if index < 0 {
			t.Fatalf("%s: expected %s in %#v", raw, want, args)
		}
		if index+1 < len(args) && !strings.HasPrefix(args[index+1], "--") {
			t.Fatalf("%s: %s takes no value, but %q follows it and becomes a stray positional", raw, want, args[index+1])
		}
	}
}

func TestLlamaSpeculativeDraftTypesJoinSpecTypeList(t *testing.T) {
	cases := []struct {
		name     string
		metadata catalog.RuntimeConfig
		want     string
	}{
		{"dflash alone", catalog.RuntimeConfig{DraftDFlash: true}, "draft-dflash"},
		{"both drafts", catalog.RuntimeConfig{DraftDFlash: true, DraftDSpark: true}, "draft-dflash,draft-dspark"},
		{"none is replaced", catalog.RuntimeConfig{SpecType: "none", DraftDSpark: true}, "draft-dspark"},
		{"no duplicates", catalog.RuntimeConfig{SpecType: "draft-dflash, ngram-mod", DraftDFlash: true}, "draft-dflash,ngram-mod"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.metadata.ModelParam = "C:/models/text.gguf"
			args, err := RuntimeArgumentsForTest(testCase.metadata, "llama")
			if err != nil {
				t.Fatal(err)
			}
			if !containsAdjacentArguments(args, "--spec-type", testCase.want) {
				t.Fatalf("expected --spec-type %s in %#v", testCase.want, args)
			}
			for _, invalid := range []string{"--draft-dflash", "--draft-dspark", "--draft-max", "--draft-n-gpu-layers"} {
				if containsArgument(args, invalid) {
					t.Fatalf("%s is not a llama-server b11157 option, got %#v", invalid, args)
				}
			}
		})
	}
}

func TestLlamaDraftCountAndLayersUseSpecDraftFlags(t *testing.T) {
	args, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{ModelParam: "C:/models/text.gguf", DraftModel: "C:/models/draft.gguf", DraftAmount: 6, DraftGPULayers: 20}, "llama")
	if err != nil {
		t.Fatal(err)
	}
	if !containsAdjacentArguments(args, "--spec-draft-n-max", "6") || !containsAdjacentArguments(args, "--spec-draft-ngl", "20") {
		t.Fatalf("llama-server b11157 removed --draft-max and never had --draft-n-gpu-layers, got %#v", args)
	}
}

func TestLlamaVideoFFmpegDirRequiresProjector(t *testing.T) {
	dir := t.TempDir()
	configs := map[string]string{
		"vision.kcpps": `{"model_param":"C:/models/text.gguf","mmproj":"C:/models/mmproj.gguf"}`,
		"text.kcpps":   `{"model_param":"C:/models/text.gguf"}`,
	}
	for filename, content := range configs {
		if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	withFFmpeg, err := NewLlamaManager(ProcessConfig{BackendURL: "http://127.0.0.1:6010", ConfigDir: dir, VideoFFmpegDir: "C:/ffmpeg/bin"})
	if err != nil {
		t.Fatal(err)
	}
	visionArgs, err := withFFmpeg.LaunchArguments("vision.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if !containsAdjacentArguments(visionArgs, "--video-ffmpeg-dir", "C:/ffmpeg/bin") {
		t.Fatalf("multimodal runtime must receive the router's ffmpeg directory, got %#v", visionArgs)
	}
	textArgs, err := withFFmpeg.LaunchArguments("text.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if containsArgument(textArgs, "--video-ffmpeg-dir") {
		t.Fatalf("text-only runtime cannot decode video and must not receive --video-ffmpeg-dir, got %#v", textArgs)
	}
	withoutFFmpeg, err := NewLlamaManager(ProcessConfig{BackendURL: "http://127.0.0.1:6011", ConfigDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	noFFmpegArgs, err := withoutFFmpeg.LaunchArguments("vision.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if containsArgument(noFFmpegArgs, "--video-ffmpeg-dir") {
		t.Fatalf("without a probed ffmpeg the flag must be omitted, got %#v", noFFmpegArgs)
	}
}

func TestLlamaEmitsSharedKoboldTextOptions(t *testing.T) {
	metadata, err := catalog.DecodeRuntimeConfig([]byte(`{
		"model_param":"C:/models/text.gguf",
		"device":"CUDA0,CUDA1",
		"autofit":true,
		"overridekv":["tokenizer.ggml.add_bos_token=bool:false","general.name=str:probe"],
		"overridetensors":"blk\\.[0-9]+\\.ffn_.*=CPU",
		"lora":["C:/loras/style.gguf","C:/loras/tone.gguf"],
		"jinja":true,
		"pooling":"mean"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	args, err := RuntimeArgumentsForTest(metadata, "llama")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range [][2]string{
		{"--device", "CUDA0,CUDA1"},
		{"--fit", "on"},
		{"--override-kv", "tokenizer.ggml.add_bos_token=bool:false,general.name=str:probe"},
		{"--override-tensor", `blk\.[0-9]+\.ffn_.*=CPU`},
		{"--lora", "C:/loras/style.gguf,C:/loras/tone.gguf"},
		{"--pooling", "mean"},
	} {
		if !containsAdjacentArguments(args, expected[0], expected[1]) {
			t.Fatalf("expected %s %s in %#v", expected[0], expected[1], args)
		}
	}
	if !containsArgument(args, "--jinja") {
		t.Fatalf("jinja=true must reach llama-server as --jinja, got %#v", args)
	}
}

func TestLlamaKeepsUpstreamDefaultsForKoboldExportedFalseValues(t *testing.T) {
	metadata, err := catalog.DecodeRuntimeConfig([]byte(`{"model_param":"C:/models/text.gguf","jinja":false,"autofit":false}`))
	if err != nil {
		t.Fatal(err)
	}
	args, err := RuntimeArgumentsForTest(metadata, "llama")
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--no-jinja", "--jinja", "--fit"} {
		if containsArgument(args, flag) {
			t.Fatalf("KoboldCpp writes jinja=false and autofit=false as its defaults; llama-server must keep its own defaults, got %s in %#v", flag, args)
		}
	}
}

func TestLlamaLeavesSharedKoboldTextOptionsAtUpstreamDefaultsWhenUnset(t *testing.T) {
	args, err := RuntimeArgumentsForTest(catalog.RuntimeConfig{ModelParam: "C:/models/text.gguf"}, "llama")
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--device", "--fit", "--override-kv", "--override-tensor", "--lora", "--jinja", "--no-jinja", "--pooling"} {
		if containsArgument(args, flag) {
			t.Fatalf("unset option must not emit %s, got %#v", flag, args)
		}
	}
}

func TestLlamaEmbeddingRuntimeOwnsPoolingOnlyWhenConfigured(t *testing.T) {
	configured := catalog.RuntimeConfig{EmbeddingsModel: "C:/models/embed.gguf", RunEmbedSeparate: true, Pooling: "cls"}
	args, err := llamaEmbeddingArguments(configured, launchTarget{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsAdjacentArguments(args, "--pooling", "cls") {
		t.Fatalf("separate embeddings runtime must receive the configured pooling, got %#v", args)
	}
	extra := []string{"--pooling", "last", "--parallel", "2"}
	if filtered := embeddingExtraArgs(configured, extra); containsArgument(filtered, "--pooling") || !containsAdjacentArguments(filtered, "--parallel", "2") {
		t.Fatalf("configured pooling must win over extra args while unrelated args survive, got %#v", filtered)
	}
	unconfigured := catalog.RuntimeConfig{EmbeddingsModel: "C:/models/embed.gguf", RunEmbedSeparate: true}
	if filtered := embeddingExtraArgs(unconfigured, extra); !containsAdjacentArguments(filtered, "--pooling", "last") {
		t.Fatalf("without a configured pooling the operator's extra --pooling must pass through, got %#v", filtered)
	}
}

func TestLlamaEmbeddingRuntimeKeepsTextOnlyOptionsOut(t *testing.T) {
	args, err := llamaEmbeddingArguments(catalog.RuntimeConfig{
		EmbeddingsModel:  "C:/models/embed.gguf",
		RunEmbedSeparate: true,
		LoRA:             "C:/loras/style.gguf",
		OverrideKV:       "general.name=str:probe",
		Jinja:            true,
	}, launchTarget{})
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--lora", "--override-kv", "--jinja"} {
		if containsArgument(args, flag) {
			t.Fatalf("text-model option %s must not reach the separate embeddings model, got %#v", flag, args)
		}
	}
}

func indexOfArgument(args []string, expected string) int {
	for index, arg := range args {
		if arg == expected {
			return index
		}
	}
	return -1
}
