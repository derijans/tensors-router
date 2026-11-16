package native

import (
	"fmt"
	"slices"
	"strings"

	"tensors-router/internal/catalog"
)

func llamaArguments(metadata catalog.RuntimeConfig, target launchTarget) ([]string, error) {
	modelPath, err := llamaTextModelPath(metadata)
	if err != nil {
		return nil, err
	}
	args := []string{
		"--host", target.host,
		"--port", target.port,
		"--model", modelPath,
		"--alias", target.modelID,
	}
	appendRepeatedStringArg(&args, "--shard", metadata.Shard)
	appendStringArg(&args, "--mcp-servers-config", target.mcpServersPath)
	appendLlamaComputeArguments(&args, metadata)
	appendLlamaSlotArguments(&args, metadata)
	appendLlamaSpeculativeArguments(&args, metadata)
	if err := appendLlamaLoadArguments(&args, metadata); err != nil {
		return nil, err
	}
	appendLlamaModelOverrideArguments(&args, metadata)
	appendFlag(&args, "--jinja", metadata.Jinja)
	appendOptionalBoolArg(&args, "--reasoning-preserve", "--no-reasoning-preserve", metadata.ReasoningPreserve.Bool())
	appendStringArg(&args, "--rpc", metadata.RPCTargets)
	appendFlag(&args, "--embeddings", strings.TrimSpace(metadata.EmbeddingsModel) != "" && !metadata.RunEmbedSeparate)
	appendStringArg(&args, "--pooling", metadata.Pooling)
	appendStringArg(&args, "--cache-type-k", firstNonEmpty(metadata.CacheTypeK, metadata.QuantKV))
	appendStringArg(&args, "--cache-type-v", firstNonEmpty(metadata.CacheTypeV, metadata.QuantKV))
	appendLlamaMultimodalArguments(&args, metadata, target.videoFFmpegDir)
	appendLlamaServerArguments(&args, metadata)
	return args, nil
}

func llamaTextModelPath(metadata catalog.RuntimeConfig) (string, error) {
	if err := validateLlamaTextToSpeech(metadata); err != nil {
		return "", err
	}
	modelPath := metadata.TextModelPath()
	if modelPath == "" {
		return "", fmt.Errorf("llama config has no text, embedding, or multimodal model path")
	}
	return modelPath, nil
}

func validateLlamaTextToSpeech(metadata catalog.RuntimeConfig) error {
	codec := strings.TrimSpace(metadata.TTSWAVTokenizer)
	switch {
	case strings.TrimSpace(metadata.TalkerModel) != "" || strings.TrimSpace(metadata.Code2WAVModel) != "":
		return fmt.Errorf("llama.cpp removed --model-talker and --model-vocoder; load a llama-server text-to-speech model with ttsmodel and its codec mmproj as ttswavtokenizer instead of talkermodel/code2wavmodel")
	case metadata.ExplicitTextModelPath() != "" && strings.TrimSpace(metadata.TTSModel) != "":
		return fmt.Errorf("llama config cannot combine a text model with ttsmodel: llama-server loads one model per runtime")
	case metadata.StandaloneTextToSpeech() && codec == "":
		return fmt.Errorf("llama-server text-to-speech needs the codec mmproj of ttsmodel in ttswavtokenizer")
	case codec != "" && !metadata.StandaloneTextToSpeech():
		return fmt.Errorf("ttswavtokenizer is the codec mmproj of a llama-server ttsmodel and needs ttsmodel without a text model")
	case codec != "" && metadata.MMProjPath() != "":
		return fmt.Errorf("llama config cannot set both mmproj and ttswavtokenizer: llama-server loads one mmproj")
	default:
		return nil
	}
}

func llamaMultimodalProjectorPath(metadata catalog.RuntimeConfig) string {
	return firstNonEmpty(metadata.MMProjPath(), metadata.TTSWAVTokenizer)
}

func appendLlamaComputeArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendIntArg(args, "--ctx-size", metadata.ContextSize)
	appendIntArg(args, "--threads", metadata.Threads)
	appendIntArg(args, "--threads-batch", metadata.BLASThreads)
	appendIntArg(args, "--batch-size", metadata.BatchSize)
	appendIntArg(args, "--ubatch-size", metadata.UBatchSize)
	appendStringArg(args, deviceFlag, metadata.Device)
	appendIntArg(args, gpuLayersFlag, int(metadata.GPULayers))
	appendIntArg(args, "--n-cpu-ffn", positive(metadata.NCPUFFN))
	appendStringArg(args, splitModeFlag, metadata.SplitMode)
	appendStringArg(args, tensorSplitFlag, metadata.TensorSplitValue())
	appendIntArg(args, mainGPUFlag, nonNegative(metadata.MainGPU))
	if metadata.AutoFit {
		*args = append(*args, "--fit", "on")
	}
	appendOnOffArg(args, "--flash-attn", metadata.FlashAttention)
}

func appendLlamaModelOverrideArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendStringListArg(args, "--override-kv", metadata.OverrideKV)
	appendStringArg(args, "--override-tensor", metadata.OverrideTensors)
	appendStringListArg(args, "--lora", metadata.LoRA)
}

func appendLlamaSlotArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendIntArg(args, "--parallel", metadata.Parallel)
	appendOptionalBoolArg(args, "--cont-batching", "--no-cont-batching", metadata.ContBatching)
	appendIntArg(args, "--cache-ram", metadata.CacheRAM)
	appendIntArg(args, "--ctx-checkpoints", metadata.CtxCheckpoints)
	appendOptionalBoolArg(args, "--kv-unified", "--no-kv-unified", metadata.KVUnified)
	appendIntArg(args, "--kv-unified-per-slot", positive(metadata.KVUnifiedPerSlot))
	appendOptionalBoolArg(args, "--cache-idle-slots", "--no-cache-idle-slots", metadata.CacheIdleSlots)
	appendFlag(args, "--swa-full", metadata.SWAFull)
	appendIntArg(args, "--moe-cache-mib", positive(metadata.MoECacheMiB))
	appendFloatArg(args, "--moe-cache-cpu-share", metadata.MoECacheCPUShare)
}

func appendLlamaSpeculativeArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendStringArg(args, "--spec-type", strings.Join(llamaSpeculativeTypes(metadata), ","))
	appendStringArg(args, "--spec-draft-type-k", metadata.SpecDraftTypeK)
	appendStringArg(args, "--spec-draft-type-v", metadata.SpecDraftTypeV)
	appendFloatArg(args, "--spec-draft-p-min", metadata.SpecDraftPMin)
	appendIntArg(args, "--spec-draft-n-ubatch", positive(metadata.SpecDraftNUBatch))
	appendStringArg(args, "--spec-draft-sampling", metadata.SpecDraftSampling)
	appendStringArg(args, "--model-draft", metadata.DraftModel)
	appendIntArg(args, "--spec-draft-n-max", metadata.DraftAmount)
	appendIntArg(args, "--spec-draft-ngl", int(metadata.DraftGPULayers))
}

func llamaSpeculativeTypes(metadata catalog.RuntimeConfig) []string {
	requested := append(strings.Split(metadata.SpecType, ","), draftSpeculativeTypes(metadata)...)
	types := make([]string, 0, len(requested))
	for _, value := range requested {
		value = strings.TrimSpace(value)
		if value != "" && !slices.Contains(types, value) {
			types = append(types, value)
		}
	}
	if len(types) > 1 {
		types = slices.DeleteFunc(types, func(value string) bool { return value == "none" })
	}
	return types
}

func draftSpeculativeTypes(metadata catalog.RuntimeConfig) []string {
	types := make([]string, 0, 2)
	if metadata.DraftDFlash {
		types = append(types, "draft-dflash")
	}
	if metadata.DraftDSpark {
		types = append(types, "draft-dspark")
	}
	return types
}

func appendLlamaLoadArguments(args *[]string, metadata catalog.RuntimeConfig) error {
	loadMode, err := metadata.LlamaLoadMode()
	if err != nil {
		return err
	}
	*args = append(*args, "--load-mode", loadMode)
	appendStringArg(args, "--lazy-mode", metadata.LazyMode)
	return nil
}

func appendLlamaMultimodalArguments(args *[]string, metadata catalog.RuntimeConfig, videoFFmpegDir string) {
	mmproj := metadata.MMProjPath()
	appendStringArg(args, "--mmproj", llamaMultimodalProjectorPath(metadata))
	appendFlag(args, "--no-mmproj-offload", metadata.MMProjCPU)
	appendOptionalBoolArg(args, "--mmproj-auto", "--no-mmproj-auto", metadata.MMProjAuto)
	appendIntArg(args, "--image-min-tokens", positive(metadata.VisionMinTokens))
	appendIntArg(args, "--image-max-tokens", positive(metadata.VisionMaxTokens))
	appendStringArg(args, "--mmproj-device", metadata.MMProjDevice)
	appendFloatArg(args, "--video-fps", metadata.VideoFPS)
	appendIntArg(args, "--video-timestamp-interval", positive(metadata.VideoTimestampInterval))
	if mmproj != "" {
		appendStringArg(args, "--video-ffmpeg-dir", videoFFmpegDir)
	}
}

func appendLlamaServerArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendStringArg(args, "--api-key-file", metadata.APIKeyFile)
	appendStringArg(args, "--log-prompts-dir", metadata.LogPromptsDir)
	appendStringArg(args, "--reasoning-effort", metadata.ReasoningEffort)
	appendStringArg(args, "--tools-runtime", metadata.ToolsRuntime)
	appendFlag(args, "--agent", metadata.Agent)
	appendFlag(args, "--media-allow-private-urls", metadata.MediaAllowPrivateURLs)
	appendStringArg(args, "--models-dir", metadata.ModelsDir)
	appendStringArg(args, "--models-preset", metadata.ModelsPreset)
	appendIntArg(args, "--models-max", metadata.ModelsMax)
	appendOptionalBoolArg(args, "--models-autoload", "--no-models-autoload", metadata.ModelsAutoload)
	appendIntArg(args, "--sse-ping-interval", metadata.SSEPingInterval)
}
