package cook

import (
	"encoding/json"
	"sort"
	"strings"

	"tensors-router/internal/backendmode"
)

const (
	LaneCommon     = "common"
	LaneRuntime    = "runtime"
	LaneText       = KindText
	LaneImage      = KindImage
	LaneEmbeddings = KindEmbeddings
	LaneMultimodal = "multimodal"
	LaneVoice      = "voice"
	LaneMusic      = "music"
)

const (
	ValueString = "string"
	ValueNumber = "number"
	ValueBool   = "bool"
	ValueJSON   = "json"
)

const (
	SectionLLM     = "llm"
	SectionImage   = "image"
	SectionEmbed   = "embed"
	SectionVoice   = "voice"
	SectionMusic   = "music"
	SectionRuntime = "runtime"
	SectionOther   = "other"
)

const (
	SourceKoboldCPP             = "https://github.com/LostRuins/koboldcpp/blob/concedo/koboldcpp.py"
	SourceKoboldCPPWiki         = "https://github.com/LostRuins/koboldcpp/wiki"
	SourceLlamaCPPServer        = "https://github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md"
	SourceStableDiffusionCPP    = "https://github.com/leejet/stable-diffusion.cpp/blob/master/examples/server/README.md"
	SourceStableDiffusionCLICPP = "https://github.com/leejet/stable-diffusion.cpp/blob/master/examples/cli/README.md"
)

var optionCatalog = enrichOptionCatalog([]OptionDefinition{
	option(backendmode.Key, "Backend", LaneRuntime, ValueString, "", false, backendmode.Kobold, backendmode.LlamaSDCPP),
	option("baseconfig", "Base Config", LaneRuntime, ValueString, "", false, "kobold"),
	option("config", "Config", LaneRuntime, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("host", "Host", LaneRuntime, ValueString, "--host", false, "kobold", "llama_sdcpp"),
	option("port", "Port", LaneRuntime, ValueNumber, "--port", false, "kobold", "llama_sdcpp"),
	option("quiet", "Quiet", LaneRuntime, ValueBool, "", false, "kobold", "llama_sdcpp"),
	option("launch", "Launch", LaneRuntime, ValueBool, "", false, "kobold"),
	option("showgui", "Show GUI", LaneRuntime, ValueBool, "", false, "kobold"),
	option("skiplauncher", "Skip Launcher", LaneRuntime, ValueBool, "", false, "kobold"),
	option("admin", "Admin", LaneRuntime, ValueBool, "", false, "kobold"),
	option("adminpassword", "Admin Password", LaneRuntime, ValueString, "", false, "kobold"),
	option("admindir", "Admin Directory", LaneRuntime, ValueString, "", false, "kobold"),
	option("adminunloadtimeout", "Admin Unload Timeout", LaneRuntime, ValueNumber, "", false, "kobold"),
	option("routermode", "Router Mode", LaneRuntime, ValueString, "", false, "kobold"),
	option("autoswapmode", "Auto Swap Mode", LaneRuntime, ValueBool, "", false, "kobold"),
	option("reqtimeout", "Request Timeout", LaneRuntime, ValueNumber, "", false, "kobold"),
	option("password", "Password", LaneRuntime, ValueString, "", false, "kobold"),
	option("ssl", "SSL", LaneRuntime, ValueBool, "", false, "kobold"),
	option("nocertify", "No Certify", LaneRuntime, ValueBool, "", false, "kobold"),
	option("remotetunnel", "Remote Tunnel", LaneRuntime, ValueBool, "", false, "kobold"),
	option("multiuser", "Multi User", LaneRuntime, ValueBool, "", false, "kobold", "llama_sdcpp"),
	option("multiplayer", "Multiplayer", LaneRuntime, ValueBool, "", false, "kobold"),
	option("websearch", "Web Search", LaneRuntime, ValueBool, "", false, "kobold"),
	option("maxrequestsize", "Max Request Size", LaneRuntime, ValueNumber, "", false, "kobold"),
	option("onready", "On Ready", LaneRuntime, ValueString, "", false, "kobold"),
	option("preloadstory", "Preload Story", LaneRuntime, ValueString, "", false, "kobold"),
	option("savedatafile", "Save Data File", LaneRuntime, ValueString, "", false, "kobold"),
	option("mcpfile", "MCP File", LaneRuntime, ValueString, "", false, "kobold"),
	option("downloaddir", "Download Directory", LaneRuntime, ValueString, "", false, "kobold"),
	option("parallelrequests", "Parallel Requests", LaneRuntime, ValueNumber, "", false, "kobold"),
	option("gendefaults", "Generation Defaults", LaneRuntime, ValueJSON, "", false, "kobold"),
	option("gendefaultsoverwrite", "Overwrite Generation Defaults", LaneRuntime, ValueBool, "", false, "kobold"),
	option("defaultgenamt", "Default Generation Amount", LaneRuntime, ValueNumber, "", false, "kobold"),
	option("genlimit", "Generation Limit", LaneRuntime, ValueNumber, "", false, "kobold"),
	option("promptlimit", "Prompt Limit", LaneRuntime, ValueNumber, "", false, "kobold"),
	option("ratelimit", "Rate Limit", LaneRuntime, ValueNumber, "", false, "kobold"),
	option("debugmode", "Debug Mode", LaneRuntime, ValueBool, "", false, "kobold"),
	option("device", "Device", LaneRuntime, ValueString, "--device", false, "kobold", "llama_sdcpp"),
	option("rpcmode", "RPC Mode", LaneRuntime, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("rpcport", "RPC Port", LaneRuntime, ValueNumber, "", false, "kobold", "llama_sdcpp"),
	option("rpchost", "RPC Host", LaneRuntime, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("rpcdevice", "RPC Device", LaneRuntime, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("rpctargets", "RPC Targets", LaneRuntime, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("model", "Model", LaneText, ValueJSON, "", false, "kobold", "llama_sdcpp"),
	option("model_param", "Model Path", LaneText, ValueString, "--model", false, "kobold", "llama_sdcpp"),
	option("nomodel", "No Model", LaneText, ValueBool, "", false, "kobold", "llama_sdcpp"),
	option("contextsize", "Context Size", LaneText, ValueNumber, "--ctx-size", false, "kobold", "llama_sdcpp"),
	option("threads", "Threads", LaneText, ValueNumber, "--threads", false, "kobold", "llama_sdcpp"),
	option("blasthreads", "BLAS Threads", LaneText, ValueNumber, "--threads-batch", false, "kobold", "llama_sdcpp"),
	option("batchsize", "Batch Size", LaneText, ValueNumber, "--batch-size", false, "kobold", "llama_sdcpp"),
	option("gpulayers", "GPU Layers", LaneText, ValueString, "--n-gpu-layers", false, "kobold", "llama_sdcpp"),
	option("splitmode", "Split Mode", LaneText, ValueString, "--split-mode", false, "kobold", "llama_sdcpp"),
	option("tensor_split", "Tensor Split", LaneText, ValueJSON, "--tensor-split", false, "kobold", "llama_sdcpp"),
	option("maingpu", "Main GPU", LaneText, ValueString, "--main-gpu", false, "kobold", "llama_sdcpp"),
	option("usemmap", "Use MMap", LaneText, ValueBool, "--no-mmap", false, "kobold", "llama_sdcpp"),
	option("usemlock", "Use MLock", LaneText, ValueBool, "--mlock", false, "kobold", "llama_sdcpp"),
	option("quantkv", "KV Cache Quant", LaneText, ValueString, "--cache-type-k", false, "kobold", "llama_sdcpp"),
	option("cache_type_k", "Cache Type K", LaneText, ValueString, "--cache-type-k", false, "llama_sdcpp"),
	option("cache_type_v", "Cache Type V", LaneText, ValueString, "--cache-type-v", false, "llama_sdcpp"),
	option("parallel", "Parallel Slots", LaneText, ValueNumber, "--parallel", false, "llama_sdcpp"),
	option("cont_batching", "Continuous Batching", LaneText, ValueBool, "--cont-batching", false, "llama_sdcpp"),
	option("ubatchsize", "Micro Batch Size", LaneText, ValueNumber, "--ubatch-size", false, "llama_sdcpp"),
	option("cache_ram", "Prompt Cache RAM", LaneText, ValueNumber, "--cache-ram", false, "llama_sdcpp"),
	option("ctx_checkpoints", "Context Checkpoints", LaneText, ValueNumber, "--ctx-checkpoints", false, "llama_sdcpp"),
	option("kv_unified", "Unified KV", LaneText, ValueBool, "--kv-unified", false, "llama_sdcpp"),
	option("cache_idle_slots", "Cache Idle Slots", LaneText, ValueBool, "--cache-idle-slots", false, "llama_sdcpp"),
	option("swa_full", "Full SWA Cache", LaneText, ValueBool, "--swa-full", false, "llama_sdcpp"),
	option("usecuda", "Use CUDA", LaneText, ValueBool, "", false, "kobold"),
	option("usecublas", "Use cuBLAS", LaneText, ValueBool, "", true, "kobold"),
	option("usevulkan", "Use Vulkan", LaneText, ValueBool, "", false, "kobold"),
	option("usecpu", "Use CPU", LaneText, ValueBool, "", false, "kobold"),
	option("flashattention", "Flash Attention", LaneText, ValueBool, "--flash-attn", false, "kobold", "llama_sdcpp"),
	option("noflashattention", "No Flash Attention", LaneText, ValueBool, "--flash-attn", false, "kobold"),
	option("lowvram", "Low VRAM", LaneText, ValueBool, "", false, "kobold"),
	option("nommq", "No MMQ", LaneText, ValueBool, "", false, "kobold"),
	option("autofit", "Auto Fit", LaneText, ValueBool, "--fit", false, "kobold", "llama_sdcpp"),
	option("blasbatchsize", "BLAS Batch Size", LaneText, ValueNumber, "", false, "kobold"),
	option("reasoningeffort", "Reasoning Effort", LaneText, ValueString, "", false, "kobold"),
	option("usemtp", "Use MTP", LaneText, ValueBool, "", false, "kobold"),
	option("swapadding", "Swap Adding", LaneText, ValueBool, "", false, "kobold"),
	option("useswa", "Use SWA", LaneText, ValueBool, "", false, "kobold"),
	option("noswa", "No SWA", LaneText, ValueBool, "", false, "kobold"),
	option("smartcache", "Smart Cache", LaneText, ValueBool, "", false, "kobold"),
	option("smartcontext", "Smart Context", LaneText, ValueBool, "", false, "kobold"),
	option("contbatch", "Continuous Batch", LaneText, ValueBool, "", false, "kobold"),
	option("pipelineparallel", "Pipeline Parallel", LaneText, ValueBool, "", false, "kobold"),
	option("nopipelineparallel", "No Pipeline Parallel", LaneText, ValueBool, "", false, "kobold"),
	option("chatcompletionsadapter", "Chat Completions Adapter", LaneText, ValueString, "", false, "kobold"),
	option("moecpu", "MoE CPU", LaneText, ValueBool, "", false, "kobold"),
	option("moeexperts", "MoE Experts", LaneText, ValueNumber, "", false, "kobold"),
	option("nobostoken", "No BOS Token", LaneText, ValueBool, "", false, "kobold"),
	option("ropeconfig", "RoPE Config", LaneText, ValueJSON, "", false, "kobold", "llama_sdcpp"),
	option("overridenativecontext", "Override Native Context", LaneText, ValueNumber, "", false, "kobold"),
	option("overridekv", "Override KV", LaneText, ValueJSON, "--override-kv", false, "kobold", "llama_sdcpp"),
	option("overridetensors", "Override Tensors", LaneText, ValueString, "--override-tensor", false, "kobold", "llama_sdcpp"),
	option("lora", "LoRA", LaneText, ValueJSON, "--lora", false, "kobold", "llama_sdcpp"),
	option("loramult", "LoRA Multiplier", LaneText, ValueJSON, "", false, "kobold"),
	option("draftmodel", "Draft Model", LaneText, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("draftamount", "Draft Amount", LaneText, ValueNumber, "", false, "kobold", "llama_sdcpp"),
	option("draftgpulayers", "Draft GPU Layers", LaneText, ValueNumber, "", false, "kobold", "llama_sdcpp"),
	option("draftgpusplit", "Draft GPU Split", LaneText, ValueJSON, "", false, "kobold", "llama_sdcpp"),
	option("spec_type", "Speculative Type", LaneText, ValueString, "--spec-type", false, "llama_sdcpp"),
	option("spec_draft_type_k", "Draft Cache Type K", LaneText, ValueString, "--spec-draft-type-k", false, "llama_sdcpp"),
	option("spec_draft_type_v", "Draft Cache Type V", LaneText, ValueString, "--spec-draft-type-v", false, "llama_sdcpp"),
	option("jinja", "Jinja", LaneText, ValueBool, "--jinja", false, "kobold", "llama_sdcpp"),
	option("jinja_tools", "Jinja Tools", LaneText, ValueBool, "", false, "kobold", "llama_sdcpp"),
	option("jinja_kwargs", "Jinja Kwargs", LaneText, ValueJSON, "", false, "kobold", "llama_sdcpp"),
	option("jinjatemplate", "Jinja Template", LaneText, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("jinjathink", "Jinja Think", LaneText, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("mmproj", "Multimodal Projector", LaneMultimodal, ValueJSON, "--mmproj", false, "kobold", "llama_sdcpp"),
	option("mmprojcpu", "Projector On CPU", LaneMultimodal, ValueBool, "--no-mmproj-offload", false, "kobold", "llama_sdcpp"),
	option("visionmaxres", "Vision Max Resolution", LaneMultimodal, ValueNumber, "", false, "kobold", "llama_sdcpp"),
	option("visionmintokens", "Vision Min Tokens", LaneMultimodal, ValueNumber, "--image-min-tokens", false, "kobold", "llama_sdcpp"),
	option("visionmaxtokens", "Vision Max Tokens", LaneMultimodal, ValueNumber, "--image-max-tokens", false, "kobold", "llama_sdcpp"),
	option("embeddingsmodel", "Embeddings Model", LaneEmbeddings, ValueString, "--model", false, "kobold", "llama_sdcpp"),
	option("embeddingsmaxctx", "Embeddings Max Context", LaneEmbeddings, ValueNumber, "", false, "kobold", "llama_sdcpp"),
	option("embeddingsgpu", "Embeddings GPU", LaneEmbeddings, ValueBool, "", false, "kobold", "llama_sdcpp"),
	option("pooling", "Pooling", LaneEmbeddings, ValueString, "--pooling", false, "llama_sdcpp"),
	option("api_key_file", "API Key File", LaneRuntime, ValueString, "--api-key-file", false, "llama_sdcpp"),
	option("log_prompts_dir", "Prompt Log Directory", LaneRuntime, ValueString, "--log-prompts-dir", false, "llama_sdcpp"),
	option("agent", "Agent Mode", LaneRuntime, ValueBool, "--agent", false, "llama_sdcpp"),
	option("models_dir", "Models Directory", LaneRuntime, ValueString, "--models-dir", false, "llama_sdcpp"),
	option("models_preset", "Models Preset", LaneRuntime, ValueString, "--models-preset", false, "llama_sdcpp"),
	option("models_max", "Max Loaded Models", LaneRuntime, ValueNumber, "--models-max", false, "llama_sdcpp"),
	option("models_autoload", "Models Autoload", LaneRuntime, ValueBool, "--models-autoload", false, "llama_sdcpp"),
	option("sdmodel", "Image Model", LaneImage, ValueString, "--model", false, "kobold", "llama_sdcpp"),
	option("sddiffusionmodel", "Diffusion Model", LaneImage, ValueString, "--diffusion-model", false, "llama_sdcpp"),
	option("sdhighnoisediffusionmodel", "High Noise Diffusion Model", LaneImage, ValueString, "--high-noise-diffusion-model", false, "llama_sdcpp"),
	option("sdunconddiffusionmodel", "Unconditional Diffusion Model", LaneImage, ValueString, "--uncond-diffusion-model", false, "llama_sdcpp"),
	option("sdllm", "Image LLM", LaneImage, ValueString, "--llm", false, "llama_sdcpp"),
	option("sdllmvision", "Image LLM Vision", LaneImage, ValueString, "--llm-vision", false, "llama_sdcpp"),
	option("sdclipvision", "CLIP Vision", LaneImage, ValueString, "--clip-vision", false, "llama_sdcpp"),
	option("sdembeddingsconnectors", "Embedding Connectors", LaneImage, ValueJSON, "--embeddings-connector", false, "llama_sdcpp"),
	option("sdcontrolnet", "ControlNet", LaneImage, ValueString, "--control-net", false, "llama_sdcpp"),
	option("sdpulidweights", "PuLID Weights", LaneImage, ValueString, "--pulid-weights", false, "llama_sdcpp"),
	option("sdpulididembedding", "PuLID ID Embedding", LaneImage, ValueString, "--pulid-id-embedding", false, "llama_sdcpp"),
	option("sdpulididweight", "PuLID ID Weight", LaneImage, ValueNumber, "--pulid-id-weight", false, "llama_sdcpp"),
	option("sdupscaler", "Upscaler", LaneImage, ValueString, "--upscale-model", false, "kobold", "llama_sdcpp"),
	option("sdvae", "VAE", LaneImage, ValueString, "--vae", false, "kobold", "llama_sdcpp"),
	option("sdvaeauto", "Auto VAE", LaneImage, ValueBool, "", false, "kobold"),
	option("sdaudiovae", "Audio VAE", LaneImage, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("sdt5xxl", "T5 XXL", LaneImage, ValueString, "--t5xxl", false, "kobold", "llama_sdcpp"),
	option("sdclip1", "CLIP 1", LaneImage, ValueString, "--clip_l", false, "kobold", "llama_sdcpp"),
	option("sdclip2", "CLIP 2", LaneImage, ValueString, "--clip_g", false, "kobold", "llama_sdcpp"),
	option("sdclipl", "CLIP L", LaneImage, ValueString, "--clip_l", false, "kobold", "llama_sdcpp"),
	option("sdclipg", "CLIP G", LaneImage, ValueString, "--clip_g", false, "kobold", "llama_sdcpp"),
	option("sdphotomaker", "PhotoMaker", LaneImage, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("sdlora", "Image LoRA", LaneImage, ValueJSON, "", false, "kobold", "llama_sdcpp"),
	option("sdloramodeldir", "LoRA Model Directory", LaneImage, ValueString, "--lora-model-dir", false, "llama_sdcpp"),
	option("sdhiresupscalersdir", "Hires Upscalers Directory", LaneImage, ValueString, "--upscaler-model-dir", false, "llama_sdcpp"),
	option("sdloramult", "Image LoRA Multiplier", LaneImage, ValueJSON, "", false, "kobold"),
	option("sdquant", "Image Quant", LaneImage, ValueNumber, "--type", false, "kobold", "llama_sdcpp"),
	option("sdtiledvae", "Tiled VAE", LaneImage, ValueNumber, "--vae-tiling", false, "kobold", "llama_sdcpp"),
	option("sdmaingpu", "Image Main GPU", LaneImage, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("sdvaedevice", "VAE Device", LaneImage, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("sdclipdevice", "CLIP Device", LaneImage, ValueString, "", false, "kobold", "llama_sdcpp"),
	option("sdflashattention", "Image Flash Attention", LaneImage, ValueBool, "--fa", false, "kobold", "llama_sdcpp"),
	option("sddiffusionflashattention", "Diffusion Flash Attention", LaneImage, ValueBool, "--diffusion-fa", false, "llama_sdcpp"),
	option("sddiffusionconvdirect", "Diffusion Conv Direct", LaneImage, ValueBool, "--diffusion-conv-direct", false, "llama_sdcpp"),
	option("sdvaeconvdirect", "VAE Conv Direct", LaneImage, ValueBool, "--vae-conv-direct", false, "llama_sdcpp"),
	option("sdoffloadcpu", "Offload To CPU", LaneImage, ValueBool, "--offload-to-cpu", false, "kobold", "llama_sdcpp"),
	option("sdvaecpu", "VAE On CPU", LaneImage, ValueBool, "--vae-on-cpu", false, "kobold", "llama_sdcpp"),
	option("sdclipgpu", "CLIP GPU", LaneImage, ValueBool, "", false, "kobold"),
	option("sdthreads", "Image Threads", LaneImage, ValueNumber, "--threads", false, "kobold", "llama_sdcpp"),
	option("sdconvdirect", "Conv Direct", LaneImage, ValueString, "", false, "kobold"),
	option("sdvramlimit", "Image VRAM Limit", LaneImage, ValueNumber, "", false, "kobold"),
	option("sdclamped", "SD Clamped", LaneImage, ValueBool, "", false, "kobold"),
	option("sdclampedsoft", "SD Soft Clamp", LaneImage, ValueBool, "", false, "kobold"),
	option("enableguidance", "Enable Guidance", LaneImage, ValueBool, "", false, "kobold"),
	option("sdgendefaults", "Image Generation Defaults", LaneImage, ValueJSON, "", false, "kobold"),
	option("sdconfig", "Image Config", LaneImage, ValueString, "", false, "kobold"),
	option("sampling_method", "Sampling Method", LaneImage, ValueString, "--sampling-method", false, "llama_sdcpp"),
	option("high_noise_sampling_method", "High Noise Sampling Method", LaneImage, ValueString, "--high-noise-sampling-method", false, "llama_sdcpp"),
	option("scheduler", "Scheduler", LaneImage, ValueString, "--scheduler", false, "llama_sdcpp"),
	option("sdbackend", "Image Backend", LaneImage, ValueString, "--backend", false, "llama_sdcpp"),
	option("sdparamsbackend", "Image Params Backend", LaneImage, ValueString, "--params-backend", false, "llama_sdcpp"),
	option("sdrpcservers", "Image RPC Servers", LaneImage, ValueJSON, "--rpc-servers", false, "llama_sdcpp"),
	option("sdmaxvram", "Image Max VRAM", LaneImage, ValueNumber, "--max-vram", false, "llama_sdcpp"),
	option("sdstreamlayers", "Stream Layers", LaneImage, ValueNumber, "--stream-layers", false, "llama_sdcpp"),
	option("sdtensortyperules", "Tensor Type Rules", LaneImage, ValueJSON, "--tensor-type-rules", false, "llama_sdcpp"),
	option("sdvaeformat", "VAE Format", LaneImage, ValueString, "--vae-format", false, "llama_sdcpp"),
	option("type", "Type", LaneImage, ValueString, "--type", false, "llama_sdcpp"),
	option("rng", "RNG", LaneImage, ValueString, "--rng", false, "llama_sdcpp"),
	option("sampler_rng", "Sampler RNG", LaneImage, ValueString, "--sampling-rng", false, "llama_sdcpp"),
	option("prediction", "Prediction", LaneImage, ValueString, "--prediction", false, "llama_sdcpp"),
	option("lora_apply_mode", "LoRA Apply Mode", LaneImage, ValueString, "--lora-apply-mode", false, "llama_sdcpp"),
	option("cache_mode", "Cache Mode", LaneImage, ValueString, "--cache-mode", false, "llama_sdcpp"),
	option("cache_option", "Cache Option", LaneImage, ValueString, "--cache-option", false, "llama_sdcpp"),
	option("whispermodel", "Whisper Model", LaneVoice, ValueString, "--model", false, "kobold", "llama_sdcpp"),
	option("ttsmodel", "TTS Model", LaneVoice, ValueString, "", false, "kobold"),
	option("ttswavtokenizer", "TTS WAV Tokenizer", LaneVoice, ValueString, "", false, "kobold"),
	option("talkermodel", "Talker Model", LaneVoice, ValueString, "--model", false, "llama_sdcpp"),
	option("code2wavmodel", "Code2WAV Model", LaneVoice, ValueString, "--model-vocoder", false, "llama_sdcpp"),
	option("ttsdir", "TTS Directory", LaneVoice, ValueString, "", false, "kobold"),
	option("ttsgpu", "TTS GPU", LaneVoice, ValueBool, "", false, "kobold"),
	option("ttsmaxlen", "TTS Max Length", LaneVoice, ValueNumber, "", false, "kobold"),
	option("ttsthreads", "TTS Threads", LaneVoice, ValueNumber, "", false, "kobold"),
	option("musicllm", "Music LLM", LaneMusic, ValueString, "", false, "kobold"),
	option("musicembeddings", "Music Embeddings", LaneMusic, ValueString, "", false, "kobold"),
	option("musicdiffusion", "Music Diffusion", LaneMusic, ValueString, "", false, "kobold"),
	option("musicvae", "Music VAE", LaneMusic, ValueString, "", false, "kobold"),
	option("musiclowvram", "Music Low VRAM", LaneMusic, ValueBool, "", false, "kobold"),
})

var catalogByKey = buildOptionByKey()

func option(key string, name string, lane string, valueType string, nativeFlag string, cudaOnly bool, backends ...string) OptionDefinition {
	return OptionDefinition{
		Key:        key,
		Name:       name,
		Lane:       lane,
		Section:    defaultSection(lane),
		ValueType:  valueType,
		Backends:   backends,
		NativeFlag: nativeFlag,
		CUDAOnly:   cudaOnly,
		Known:      true,
	}
}

func OptionCatalog() []OptionDefinition {
	values := append([]OptionDefinition{}, optionCatalog...)
	sort.Slice(values, func(left, right int) bool {
		return values[left].Key < values[right].Key
	})
	return values
}

func ObservedOptions(models []Options) []OptionDefinition {
	known := catalogByKey
	seen := map[string]struct{}{}
	values := make([]OptionDefinition, 0)
	for _, options := range models {
		for key := range options {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, ok := known[key]; ok {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			values = append(values, OptionDefinition{
				Key:       key,
				Name:      titleFromKey(key),
				Lane:      LaneCommon,
				Section:   SectionOther,
				ValueType: ValueJSON,
				Known:     false,
			})
		}
	}
	sort.Slice(values, func(left, right int) bool {
		return values[left].Key < values[right].Key
	})
	return values
}

func FilterOptionsForKinds(options Options, components []Component) Options {
	if len(options) == 0 {
		return nil
	}
	allowed := lanesForComponents(components)
	filtered := Options{}
	for key, value := range options {
		definition, ok := catalogByKey[key]
		if !ok || definition.Lane == LaneCommon || definition.Lane == LaneRuntime || allowed[definition.Lane] {
			filtered[key] = cloneRaw(value)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func OptionDefinitionForKey(key string) (OptionDefinition, bool) {
	value, ok := catalogByKey[strings.TrimSpace(key)]
	return value, ok
}

func IsCUDAOnlyOption(key string) bool {
	definition, ok := OptionDefinitionForKey(key)
	return ok && definition.CUDAOnly
}

func BackendModeOption(options Options) (string, bool, error) {
	value, ok := options[backendmode.Key]
	if !ok || len(value) == 0 || strings.EqualFold(strings.TrimSpace(string(value)), "null") {
		return "", false, nil
	}
	var mode string
	if err := json.Unmarshal(value, &mode); err != nil {
		return "", true, err
	}
	mode = backendmode.Normalize(mode)
	if mode == "" {
		return "", false, nil
	}
	if !backendmode.Valid(mode) {
		_, err := backendmode.Resolve(mode, "")
		return "", true, err
	}
	return mode, true, nil
}

func lanesForComponents(components []Component) map[string]bool {
	allowed := map[string]bool{}
	for _, component := range components {
		switch component.Kind {
		case KindText:
			allowed[LaneText] = true
			allowed[LaneMultimodal] = true
		case KindImage:
			allowed[LaneImage] = true
		case KindEmbeddings:
			allowed[LaneEmbeddings] = true
			allowed[LaneText] = true
		case KindVoice:
			allowed[LaneVoice] = true
		case KindMusic:
			allowed[LaneMusic] = true
		}
	}
	return allowed
}

func buildOptionByKey() map[string]OptionDefinition {
	values := make(map[string]OptionDefinition, len(optionCatalog))
	for _, definition := range optionCatalog {
		values[definition.Key] = definition
	}
	return values
}

type optionMetadata struct {
	choices      []string
	modelRole    string
	defaultValue string
	source       string
	section      string
}

func enrichOptionCatalog(values []OptionDefinition) []OptionDefinition {
	for index := range values {
		metadata := optionMetadataByKey[values[index].Key]
		if metadata.section != "" {
			values[index].Section = metadata.section
		}
		if len(metadata.choices) > 0 {
			values[index].Choices = append([]string{}, metadata.choices...)
		}
		values[index].ModelRole = metadata.modelRole
		values[index].Default = metadata.defaultValue
		values[index].Source = metadata.source
		if values[index].Source == "" {
			values[index].Source = defaultSource(values[index].Backends)
		}
	}
	return values
}

var optionMetadataByKey = map[string]optionMetadata{
	"backend_mode":               meta(values(backendmode.Kobold, backendmode.LlamaSDCPP), "", "", "", ""),
	"baseconfig":                 meta(nil, "config", "", SourceKoboldCPP, ""),
	"config":                     meta(nil, "config", "", SourceLlamaCPPServer, ""),
	"host":                       meta(nil, "", "127.0.0.1", SourceLlamaCPPServer, ""),
	"port":                       meta(nil, "", "5001", SourceLlamaCPPServer, ""),
	"quiet":                      meta(boolChoices(), "", "true", SourceKoboldCPP, ""),
	"launch":                     meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"showgui":                    meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"skiplauncher":               meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"admin":                      meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"autoswapmode":               meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"ssl":                        meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"nocertify":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"remotetunnel":               meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"multiuser":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"multiplayer":                meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"websearch":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"device":                     meta(values("none", "cuda", "vulkan", "rocm", "metal", "cpu"), "", "", SourceLlamaCPPServer, ""),
	"rpcmode":                    meta(values("disabled", "connect", "host"), "", "disabled", SourceKoboldCPP, ""),
	"model":                      meta(nil, "llm", "", SourceKoboldCPP, ""),
	"model_param":                meta(nil, "llm", "", SourceLlamaCPPServer, ""),
	"nomodel":                    meta(boolChoices(), "", "false", SourceKoboldCPP, ""),
	"contextsize":                meta(values("4096", "8192", "16384", "32768", "65536"), "", "4096", SourceKoboldCPP, ""),
	"threads":                    meta(values("-1", "1", "2", "4", "8", "16", "32"), "", "-1", SourceLlamaCPPServer, ""),
	"blasthreads":                meta(values("-1", "1", "2", "4", "8", "16", "32"), "", "-1", SourceLlamaCPPServer, ""),
	"batchsize":                  meta(values("-1", "16", "32", "64", "128", "256", "512", "1024", "2048", "4096"), "", "512", SourceLlamaCPPServer, ""),
	"gpulayers":                  meta(values("-1", "0", "auto", "all"), "", "auto", SourceLlamaCPPServer, ""),
	"splitmode":                  meta(values("none", "layer", "row", "tensor"), "", "layer", SourceLlamaCPPServer, ""),
	"tensor_split":               meta(nil, "", "", SourceLlamaCPPServer, ""),
	"maingpu":                    meta(values("0", "1", "2", "3", "main", "CPU"), "", "0", SourceLlamaCPPServer, ""),
	"usemmap":                    meta(boolChoices(), "", "true", SourceLlamaCPPServer, ""),
	"usemlock":                   meta(boolChoices(), "", "false", SourceLlamaCPPServer, ""),
	"quantkv":                    meta(values("f16", "bf16", "q8_0", "q5_1", "q4_0", "0", "1", "2", "3"), "", "f16", SourceKoboldCPP, ""),
	"cache_type_k":               meta(cacheTypeChoices(), "", "f16", SourceLlamaCPPServer, ""),
	"cache_type_v":               meta(cacheTypeChoices(), "", "f16", SourceLlamaCPPServer, ""),
	"usecuda":                    meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"usecublas":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"usevulkan":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"usecpu":                     meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"flashattention":             meta(boolChoices(), "", "false", SourceLlamaCPPServer, ""),
	"noflashattention":           meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"lowvram":                    meta(boolChoices(), "", "false", SourceKoboldCPP, ""),
	"nommq":                      meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"autofit":                    meta(boolChoices(), "", "true", SourceLlamaCPPServer, ""),
	"lora":                       meta(nil, "lora", "", SourceLlamaCPPServer, ""),
	"draftmodel":                 meta(nil, "llm", "", SourceLlamaCPPServer, ""),
	"jinja":                      meta(boolChoices(), "", "", SourceLlamaCPPServer, ""),
	"jinja_tools":                meta(boolChoices(), "", "", SourceLlamaCPPServer, ""),
	"jinjathink":                 meta(values("default", "none", "openai", "deepseek", "qwen"), "", "default", SourceKoboldCPP, ""),
	"mmproj":                     meta(nil, "multimodal", "", SourceLlamaCPPServer, SectionEmbed),
	"mmprojcpu":                  meta(boolChoices(), "", "false", SourceLlamaCPPServer, SectionEmbed),
	"visionmaxres":               meta(values("512", "768", "1024", "1536", "2048"), "", "1024", SourceKoboldCPP, SectionEmbed),
	"visionmintokens":            meta(values("-1"), "", "-1", SourceLlamaCPPServer, SectionEmbed),
	"visionmaxtokens":            meta(values("-1"), "", "-1", SourceLlamaCPPServer, SectionEmbed),
	"embeddingsmodel":            meta(nil, "embeddings", "", SourceKoboldCPP, SectionEmbed),
	"embeddingsmaxctx":           meta(values("512", "1024", "2048", "4096", "8192"), "", "4096", SourceKoboldCPP, SectionEmbed),
	"embeddingsgpu":              meta(boolChoices(), "", "", SourceKoboldCPP, SectionEmbed),
	"pooling":                    meta(values("none", "mean", "cls", "last", "rank"), "", "", SourceLlamaCPPServer, SectionEmbed),
	"sdmodel":                    meta(nil, "image", "", SourceStableDiffusionCPP, ""),
	"sdupscaler":                 meta(nil, "upscaler", "", SourceStableDiffusionCPP, ""),
	"sdvae":                      meta(nil, "vae", "", SourceStableDiffusionCPP, ""),
	"sdvaeauto":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"sdaudiovae":                 meta(nil, "vae", "", SourceKoboldCPP, ""),
	"sdt5xxl":                    meta(nil, "t5", "", SourceStableDiffusionCPP, ""),
	"sdclip1":                    meta(nil, "clip", "", SourceStableDiffusionCPP, ""),
	"sdclip2":                    meta(nil, "clip", "", SourceStableDiffusionCPP, ""),
	"sdclipl":                    meta(nil, "clip", "", SourceStableDiffusionCPP, ""),
	"sdclipg":                    meta(nil, "clip", "", SourceStableDiffusionCPP, ""),
	"sdphotomaker":               meta(nil, "image", "", SourceStableDiffusionCPP, ""),
	"sdlora":                     meta(nil, "lora", "", SourceStableDiffusionCPP, ""),
	"sdquant":                    meta(values("0", "1", "2"), "", "0", SourceKoboldCPP, ""),
	"sdtiledvae":                 meta(values("0", "512", "768", "1024"), "", "0", SourceStableDiffusionCPP, ""),
	"sdmaingpu":                  meta(values("0", "1", "2", "3", "main", "CPU"), "", "main", SourceKoboldCPP, ""),
	"sdvaedevice":                meta(values("0", "1", "2", "3", "main", "CPU"), "", "main", SourceKoboldCPP, ""),
	"sdclipdevice":               meta(values("0", "1", "2", "3", "main", "CPU"), "", "CPU", SourceKoboldCPP, ""),
	"sdflashattention":           meta(boolChoices(), "", "false", SourceStableDiffusionCPP, ""),
	"sdoffloadcpu":               meta(boolChoices(), "", "false", SourceStableDiffusionCPP, ""),
	"sdvaecpu":                   meta(boolChoices(), "", "", SourceStableDiffusionCPP, ""),
	"sdclipgpu":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"sdthreads":                  meta(values("-1", "1", "2", "4", "8", "16", "32"), "", "-1", SourceStableDiffusionCPP, ""),
	"sdconvdirect":               meta(values("off", "full", "vaeonly"), "", "off", SourceKoboldCPP, ""),
	"sampling_method":            meta(samplingMethodChoices(), "", "euler", SourceStableDiffusionCPP, ""),
	"high_noise_sampling_method": meta(samplingMethodChoices(), "", "euler", SourceStableDiffusionCPP, ""),
	"scheduler":                  meta(schedulerChoices(), "", "", SourceStableDiffusionCPP, ""),
	"type":                       meta(values("f32", "f16", "q4_0", "q4_1", "q5_0", "q5_1", "q8_0", "q2_K", "q3_K", "q4_K"), "", "", SourceStableDiffusionCPP, ""),
	"rng":                        meta(values("std_default", "cuda", "cpu"), "", "", SourceStableDiffusionCPP, ""),
	"sampler_rng":                meta(values("std_default", "cuda", "cpu"), "", "", SourceStableDiffusionCPP, ""),
	"prediction":                 meta(values("eps", "v", "edm_v", "sd3_flow", "flux_flow", "flux2_flow"), "", "", SourceStableDiffusionCPP, ""),
	"lora_apply_mode":            meta(values("auto", "immediately", "at_runtime"), "", "auto", SourceStableDiffusionCPP, ""),
	"cache_mode":                 meta(values("easycache", "ucache", "dbcache", "taylorseer", "cache-dit", "spectrum"), "", "", SourceStableDiffusionCPP, ""),
	"whispermodel":               meta(nil, "voice", "", SourceKoboldCPP, ""),
	"ttsmodel":                   meta(nil, "voice", "", SourceKoboldCPP, ""),
	"ttswavtokenizer":            meta(nil, "voice", "", SourceKoboldCPP, ""),
	"ttsdir":                     meta(nil, "voice", "", SourceKoboldCPP, ""),
	"ttsgpu":                     meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"ttsthreads":                 meta(values("-1", "1", "2", "4", "8", "16", "32"), "", "-1", SourceKoboldCPP, ""),
	"musicllm":                   meta(nil, "music", "", SourceKoboldCPP, ""),
	"musicembeddings":            meta(nil, "music", "", SourceKoboldCPP, ""),
	"musicdiffusion":             meta(nil, "music", "", SourceKoboldCPP, ""),
	"musicvae":                   meta(nil, "music", "", SourceKoboldCPP, ""),
	"musiclowvram":               meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"parallelrequests":           meta(values("1", "2", "4", "8", "16"), "", "", SourceKoboldCPP, ""),
	"gendefaults":                meta(nil, "", "", SourceKoboldCPP, ""),
	"gendefaultsoverwrite":       meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"defaultgenamt":              meta(values("1", "2", "4", "8"), "", "", SourceKoboldCPP, ""),
	"genlimit":                   meta(values("0", "1", "2", "4", "8", "16"), "", "", SourceKoboldCPP, ""),
	"promptlimit":                meta(values("0", "1024", "4096", "8192", "16384"), "", "", SourceKoboldCPP, ""),
	"ratelimit":                  meta(values("0", "1", "2", "4", "8", "16"), "", "", SourceKoboldCPP, ""),
	"debugmode":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"parallel":                   meta(values("1", "2", "4", "8", "16"), "", "1", SourceLlamaCPPServer, ""),
	"cont_batching":              meta(boolChoices(), "", "true", SourceLlamaCPPServer, ""),
	"ubatchsize":                 meta(values("128", "256", "512", "1024", "2048"), "", "512", SourceLlamaCPPServer, ""),
	"cache_ram":                  meta(values("-1", "0", "2048", "4096", "8192", "16384"), "", "8192", SourceLlamaCPPServer, ""),
	"ctx_checkpoints":            meta(values("0", "8", "16", "32", "64"), "", "32", SourceLlamaCPPServer, ""),
	"kv_unified":                 meta(boolChoices(), "", "", SourceLlamaCPPServer, ""),
	"cache_idle_slots":           meta(boolChoices(), "", "true", SourceLlamaCPPServer, ""),
	"swa_full":                   meta(boolChoices(), "", "false", SourceLlamaCPPServer, ""),
	"reasoningeffort":            meta(values("low", "medium", "high"), "", "", SourceKoboldCPP, ""),
	"usemtp":                     meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"swapadding":                 meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"useswa":                     meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"noswa":                      meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"smartcache":                 meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"smartcontext":               meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"contbatch":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"pipelineparallel":           meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"nopipelineparallel":         meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"chatcompletionsadapter":     meta(nil, "", "", SourceKoboldCPP, ""),
	"moecpu":                     meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"moeexperts":                 meta(values("0", "1", "2", "4", "8", "16"), "", "", SourceKoboldCPP, ""),
	"nobostoken":                 meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"spec_type":                  meta(values("none", "draft-simple", "draft-eagle3", "draft-mtp", "ngram-simple", "ngram-map-k", "ngram-map-k4v", "ngram-mod", "ngram-cache"), "", "none", SourceLlamaCPPServer, ""),
	"spec_draft_type_k":          meta(cacheTypeChoices(), "", "f16", SourceLlamaCPPServer, ""),
	"spec_draft_type_v":          meta(cacheTypeChoices(), "", "f16", SourceLlamaCPPServer, ""),
	"api_key_file":               meta(nil, "", "", SourceLlamaCPPServer, SectionRuntime),
	"log_prompts_dir":            meta(nil, "", "", SourceLlamaCPPServer, SectionRuntime),
	"agent":                      meta(boolChoices(), "", "false", SourceLlamaCPPServer, SectionRuntime),
	"models_dir":                 meta(nil, "", "", SourceLlamaCPPServer, SectionRuntime),
	"models_preset":              meta(nil, "", "", SourceLlamaCPPServer, SectionRuntime),
	"models_max":                 meta(values("0", "1", "2", "4", "8"), "", "4", SourceLlamaCPPServer, SectionRuntime),
	"models_autoload":            meta(boolChoices(), "", "true", SourceLlamaCPPServer, SectionRuntime),
	"sddiffusionmodel":           meta(nil, "image", "", SourceStableDiffusionCPP, ""),
	"sdhighnoisediffusionmodel":  meta(nil, "image", "", SourceStableDiffusionCPP, ""),
	"sdunconddiffusionmodel":     meta(nil, "image", "", SourceStableDiffusionCPP, ""),
	"sdllm":                      meta(nil, "llm", "", SourceStableDiffusionCPP, ""),
	"sdllmvision":                meta(nil, "multimodal", "", SourceStableDiffusionCPP, ""),
	"sdclipvision":               meta(nil, "clip", "", SourceStableDiffusionCPP, ""),
	"sdembeddingsconnectors":     meta(nil, "embeddings", "", SourceStableDiffusionCPP, ""),
	"sdcontrolnet":               meta(nil, "image", "", SourceStableDiffusionCPP, ""),
	"sdpulidweights":             meta(nil, "image", "", SourceStableDiffusionCPP, ""),
	"sdpulididembedding":         meta(nil, "image", "", SourceStableDiffusionCPP, ""),
	"sdpulididweight":            meta(values("0", "0.5", "1"), "", "", SourceStableDiffusionCPP, ""),
	"sdloramodeldir":             meta(nil, "lora", "", SourceStableDiffusionCPP, ""),
	"sdhiresupscalersdir":        meta(nil, "upscaler", "", SourceStableDiffusionCPP, ""),
	"sddiffusionflashattention":  meta(boolChoices(), "", "false", SourceStableDiffusionCPP, ""),
	"sddiffusionconvdirect":      meta(boolChoices(), "", "false", SourceStableDiffusionCPP, ""),
	"sdvaeconvdirect":            meta(boolChoices(), "", "false", SourceStableDiffusionCPP, ""),
	"sdvramlimit":                meta(values("0", "4096", "8192", "12288", "16384", "24576"), "", "", SourceKoboldCPP, ""),
	"sdclamped":                  meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"sdclampedsoft":              meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"enableguidance":             meta(boolChoices(), "", "", SourceKoboldCPP, ""),
	"sdgendefaults":              meta(nil, "", "", SourceKoboldCPP, ""),
	"sdconfig":                   meta(nil, "config", "", SourceKoboldCPP, ""),
	"sdbackend":                  meta(nil, "", "", SourceStableDiffusionCPP, ""),
	"sdparamsbackend":            meta(nil, "", "", SourceStableDiffusionCPP, ""),
	"sdrpcservers":               meta(nil, "", "", SourceStableDiffusionCPP, ""),
	"sdmaxvram":                  meta(values("0", "4096", "8192", "12288", "16384", "24576"), "", "", SourceStableDiffusionCPP, ""),
	"sdstreamlayers":             meta(values("0", "1", "2", "4", "8", "16"), "", "", SourceStableDiffusionCPP, ""),
	"sdtensortyperules":          meta(nil, "", "", SourceStableDiffusionCPP, ""),
	"sdvaeformat":                meta(nil, "vae", "", SourceStableDiffusionCPP, ""),
	"talkermodel":                meta(nil, "voice", "", SourceLlamaCPPServer, SectionVoice),
	"code2wavmodel":              meta(nil, "voice", "", SourceLlamaCPPServer, SectionVoice),
}

func meta(choices []string, modelRole string, defaultValue string, source string, section string) optionMetadata {
	return optionMetadata{
		choices:      choices,
		modelRole:    modelRole,
		defaultValue: defaultValue,
		source:       source,
		section:      section,
	}
}

func defaultSection(lane string) string {
	switch lane {
	case LaneText:
		return SectionLLM
	case LaneImage:
		return SectionImage
	case LaneEmbeddings, LaneMultimodal:
		return SectionEmbed
	case LaneVoice:
		return SectionVoice
	case LaneMusic:
		return SectionMusic
	case LaneRuntime:
		return SectionRuntime
	default:
		return SectionOther
	}
}

func defaultSource(backends []string) string {
	for _, backend := range backends {
		switch backend {
		case "kobold":
			return SourceKoboldCPP
		case "llama_sdcpp":
			return SourceLlamaCPPServer
		}
	}
	return ""
}

func boolChoices() []string {
	return values("true", "false")
}

func cacheTypeChoices() []string {
	return values("f32", "f16", "bf16", "q8_0", "q4_0", "q4_1", "iq4_nl", "q5_0", "q5_1")
}

func samplingMethodChoices() []string {
	return values("euler", "euler_a", "heun", "dpm2", "dpm++2s_a", "dpm++2m", "dpm++2mv2", "ipndm", "ipndm_v", "lcm", "ddim_trailing", "tcd", "res_multistep", "res_2s", "er_sde", "euler_cfg_pp", "euler_a_cfg_pp")
}

func schedulerChoices() []string {
	return values("discrete", "karras", "exponential", "ays", "gits", "smoothstep", "sgm_uniform", "simple", "kl_optimal", "lcm", "bong_tangent", "ltx2")
}

func values(items ...string) []string {
	return append([]string{}, items...)
}

func titleFromKey(key string) string {
	key = strings.ReplaceAll(key, "_", " ")
	key = strings.ReplaceAll(key, "-", " ")
	parts := strings.Fields(key)
	for index, part := range parts {
		if part == "" {
			continue
		}
		parts[index] = strings.ToUpper(part[:1]) + part[1:]
	}
	if len(parts) == 0 {
		return key
	}
	return strings.Join(parts, " ")
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	cloned := make(json.RawMessage, len(value))
	copy(cloned, value)
	return cloned
}
