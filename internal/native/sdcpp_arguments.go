package native

import (
	"fmt"
	"strconv"

	"tensors-router/internal/catalog"
)

func sdcppArguments(metadata catalog.RuntimeConfig, target launchTarget) ([]string, error) {
	modelPath := metadata.ImageModelPath()
	if modelPath == "" {
		return nil, fmt.Errorf("sd.cpp config has no image model path")
	}
	args := []string{
		"--listen-ip", target.host,
		"--listen-port", target.port,
		"--model", modelPath,
	}
	appendStringArg(&args, "--log-level", metadata.SDLogLevel)
	appendSDCPPComponentArguments(&args, metadata)
	appendSDCPPPlacementArguments(&args, metadata)
	appendSDCPPComputeArguments(&args, metadata)
	appendSDCPPGenerationDefaults(&args, metadata)
	return args, nil
}

func appendSDCPPComponentArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendStringArg(args, "--vae", metadata.SDVAE)
	appendStringArg(args, "--audio-vae", metadata.SDAudioVAE)
	appendStringArg(args, "--audio-encoder", metadata.SDAudioEncoder)
	appendStringArg(args, "--photo-maker", metadata.SDPhotoMaker)
	appendStringArg(args, "--diffusion-model", metadata.SDDiffusionModel)
	appendStringArg(args, "--high-noise-diffusion-model", metadata.SDHighNoiseDiffusionModel)
	appendStringArg(args, "--uncond-diffusion-model", metadata.SDUncondDiffusionModel)
	appendStringArg(args, "--t5xxl", metadata.SDT5XXL)
	appendStringArg(args, "--clip_l", firstNonEmpty(metadata.SDClipL, metadata.SDClip1))
	appendStringArg(args, "--clip_g", firstNonEmpty(metadata.SDClipG, metadata.SDClip2))
	appendStringArg(args, "--llm", metadata.SDLLM)
	appendStringArg(args, "--llm_vision", metadata.SDLLMVision)
	appendStringArg(args, "--tokenizer", metadata.SDTokenizer)
	appendStringArg(args, "--clip_vision", metadata.SDClipVision)
	appendStringArg(args, "--ip-adapter", metadata.SDIPAdapter)
	appendStringArg(args, "--motion-module", metadata.SDMotionModule)
	appendStringListArg(args, "--embeddings-connectors", metadata.SDEmbeddingsConnectors)
	appendStringArg(args, "--control-net", metadata.SDControlNet)
	appendStringArg(args, "--pulid-weights", metadata.SDPulidWeights)
	appendStringArg(args, "--pulid-id-embedding", metadata.SDPulidIDEmbedding)
	appendFloatArg(args, "--pulid-id-weight", metadata.SDPulidIDWeight)
	appendStringArg(args, "--upscale-model", metadata.SDUpscaler)
	appendStringArg(args, "--model-args", metadata.SDModelArgs)
}

func appendSDCPPPlacementArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendStringArg(args, "--backend", metadata.SDBackend)
	appendStringArg(args, "--params-backend", metadata.SDParamsBackend)
	appendStringListArg(args, "--rpc-servers", metadata.SDRPCServers)
	appendStringArg(args, "--max-vram", nativeSingleString(metadata.SDMaxVRAM))
	appendOnOffArg(args, "--auto-fit", metadata.SDAutoFit)
	appendStringArg(args, "--split-mode", metadata.SDSplitMode)
	appendStringListArg(args, "--tensor-type-rules", metadata.SDTensorTypeRules)
	appendStringArg(args, "--vae-format", metadata.SDVAEFormat)
	appendStringArg(args, "--lora-model-dir", metadata.SDLoRAModelDir)
	appendStringArg(args, "--hires-upscalers-dir", metadata.SDHiresUpscalersDir)
	appendIntArg(args, "--threads", metadata.SDThreads)
	appendOptionalIntArg(args, "--conditioning-cache-size", metadata.SDConditioningCacheSize)
	appendFlag(args, "--disable-prefetch", metadata.SDDisablePrefetch)
	appendFlag(args, "--disable-segmented-compute", metadata.SDDisableSegmentedCompute)
	appendFlag(args, "--offload-to-cpu", metadata.SDOffloadCPU)
	appendFlag(args, "--vae-on-cpu", metadata.SDVAECPU)
}

func appendSDCPPComputeArguments(args *[]string, metadata catalog.RuntimeConfig) {
	appendFlag(args, "--fa", metadata.SDFlashAttention)
	appendFlag(args, "--diffusion-fa", metadata.SDDiffusionFlashAttention)
	appendFlag(args, "--sage-attn", metadata.SDSageAttention)
	appendFlag(args, "--diffusion-conv-direct", metadata.SDDiffusionConvDirect)
	appendFlag(args, "--vae-conv-direct", metadata.SDVAEConvDirect)
	appendFloatArg(args, "--linear-scale", metadata.SDLinearScale)
	appendFloatArg(args, "--attn-scale", metadata.SDAttnScale)
	appendStringArg(args, "--type", metadata.SDType)
	appendStringArg(args, "--prediction", metadata.SDPrediction)
	appendStringArg(args, "--lora-apply-mode", metadata.SDLoRAApplyMode)
}

func appendSDCPPGenerationDefaults(args *[]string, metadata catalog.RuntimeConfig) {
	appendFlag(args, "--circular", metadata.SDCircular)
	appendFlag(args, "--circularx", metadata.SDCircularX)
	appendFlag(args, "--circulary", metadata.SDCircularY)
	if metadata.SDTiledVAE > 0 {
		tileSize := strconv.Itoa(metadata.SDTiledVAE) + "x" + strconv.Itoa(metadata.SDTiledVAE)
		*args = append(*args, "--vae-tiling", "--vae-tile-size", tileSize)
	}
	appendStringArg(args, "--extra-tiling-args", metadata.SDExtraTilingArgs)
	appendStringArg(args, "--sampling-method", metadata.SDSamplingMethod)
	appendStringArg(args, "--high-noise-sampling-method", metadata.SDHighNoiseSamplingMethod)
	appendStringArg(args, "--scheduler", metadata.SDScheduler)
	appendStringArg(args, "--extra-sample-args", metadata.SDExtraSampleArgs)
	appendStringArg(args, "--rng", metadata.SDRNG)
	appendStringArg(args, "--sampler-rng", metadata.SDSamplerRNG)
	appendStringArg(args, "--cache-mode", metadata.SDCacheMode)
	appendStringArg(args, "--cache-option", metadata.SDCacheOption)
	appendJoinedStringListArg(args, "--image-preprocess", metadata.SDImagePreprocess, ";")
}
