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
	if metadata.ExplicitTextModelPath() != "" && strings.TrimSpace(metadata.TTSModel) != "" {
		return "", fmt.Errorf("llama config cannot combine a text model with standalone ttsmodel")
	}
	vocoder := firstNonEmpty(metadata.Code2WAVModel, metadata.TTSWAVTokenizer)
	standaloneTTS := metadata.ExplicitTextModelPath() == "" && strings.TrimSpace(metadata.TTSModel) != ""
	if vocoder != "" || strings.TrimSpace(metadata.TalkerModel) != "" || standaloneTTS {
		return "", fmt.Errorf("llama.cpp removed --model-vocoder and --model-talker and llama-server has no text-to-speech endpoint; talkermodel/ttsmodel/ttswavtokenizer/code2wavmodel are not supported by the llama_sdcpp backend, use kobold or vllm for text-to-speech")
	}
	modelPath := metadata.TextModelPath()
	if modelPath == "" {
		return "", fmt.Errorf("llama config has no text, embedding, or multimodal model path")
	}
	return modelPath, nil
}

func appendLlamaComputeArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendIntArg(args, "--ctx-size", metadata.ContextSize)
	appendIntArg(args, "--threads", metadata.Threads)
	appendIntArg(args, "--threads-batch", metadata.BLASThreads)
	appendIntArg(args, "--batch-size", metadata.BatchSize)
	appendIntArg(args, "--ubatch-size", metadata.UBatchSize)
	appendStringArg(args, "--device", metadata.Device)
	appendIntArg(args, "--n-gpu-layers", int(metadata.GPULayers))
	appendIntArg(args, "--n-cpu-ffn", positive(metadata.NCPUFFN))
	appendStringArg(args, "--split-mode", metadata.SplitMode)
	appendStringArg(args, "--tensor-split", metadata.TensorSplitValue())
	appendIntArg(args, "--main-gpu", nonNegative(metadata.MainGPU))
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
}

func appendLlamaSpeculativeArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendStringArg(args, "--spec-type", strings.Join(llamaSpeculativeTypes(metadata), ","))
	appendStringArg(args, "--spec-draft-type-k", metadata.SpecDraftTypeK)
	appendStringArg(args, "--spec-draft-type-v", metadata.SpecDraftTypeV)
	appendFloatArg(args, "--spec-draft-p-min", metadata.SpecDraftPMin)
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
	appendStringArg(args, "--mmproj", mmproj)
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
	appendStringArg(args, "--models-dir", metadata.ModelsDir)
	appendStringArg(args, "--models-preset", metadata.ModelsPreset)
	appendIntArg(args, "--models-max", metadata.ModelsMax)
	appendOptionalBoolArg(args, "--models-autoload", "--no-models-autoload", metadata.ModelsAutoload)
	appendIntArg(args, "--sse-ping-interval", metadata.SSEPingInterval)
}
