package catalog

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

type RuntimeConfig struct {
	BackendMode               string  `json:"backend_mode"`
	RouterUnloadPolicy        string  `json:"router_unload_policy"`
	Model                     any     `json:"model"`
	ModelParam                string  `json:"model_param"`
	NoModel                   bool    `json:"nomodel"`
	Threads                   int     `json:"threads"`
	BLASThreads               int     `json:"blasthreads"`
	BatchSize                 int     `json:"batchsize"`
	UBatchSize                int     `json:"ubatchsize"`
	GPULayers                 int     `json:"gpulayers"`
	SplitMode                 string  `json:"splitmode"`
	TensorSplit               any     `json:"tensor_split"`
	MainGPU                   int     `json:"maingpu"`
	UseMMap                   bool    `json:"usemmap"`
	UseMLock                  bool    `json:"usemlock"`
	QuantKV                   string  `json:"quantkv"`
	CacheTypeK                string  `json:"cache_type_k"`
	CacheTypeV                string  `json:"cache_type_v"`
	Parallel                  int     `json:"parallel"`
	ContBatching              *bool   `json:"cont_batching"`
	CacheRAM                  int     `json:"cache_ram"`
	CtxCheckpoints            int     `json:"ctx_checkpoints"`
	KVUnified                 *bool   `json:"kv_unified"`
	CacheIdleSlots            *bool   `json:"cache_idle_slots"`
	SWAFull                   bool    `json:"swa_full"`
	SpecType                  string  `json:"spec_type"`
	SpecDraftTypeK            string  `json:"spec_draft_type_k"`
	SpecDraftTypeV            string  `json:"spec_draft_type_v"`
	APIKeyFile                string  `json:"api_key_file"`
	LogPromptsDir             string  `json:"log_prompts_dir"`
	Agent                     bool    `json:"agent"`
	ModelsDir                 string  `json:"models_dir"`
	ModelsPreset              string  `json:"models_preset"`
	ModelsMax                 int     `json:"models_max"`
	ModelsAutoload            *bool   `json:"models_autoload"`
	SDModel                   string  `json:"sdmodel"`
	SDDiffusionModel          string  `json:"sddiffusionmodel"`
	SDHighNoiseDiffusionModel string  `json:"sdhighnoisediffusionmodel"`
	SDUncondDiffusionModel    string  `json:"sdunconddiffusionmodel"`
	SDLLM                     string  `json:"sdllm"`
	SDLLMVision               string  `json:"sdllmvision"`
	SDClipVision              string  `json:"sdclipvision"`
	SDEmbeddingsConnectors    any     `json:"sdembeddingsconnectors"`
	SDControlNet              string  `json:"sdcontrolnet"`
	SDPulidWeights            string  `json:"sdpulidweights"`
	SDPulidIDEmbedding        string  `json:"sdpulididembedding"`
	SDPulidIDWeight           float64 `json:"sdpulididweight"`
	SDBackend                 string  `json:"sdbackend"`
	SDParamsBackend           string  `json:"sdparamsbackend"`
	SDRPCServers              any     `json:"sdrpcservers"`
	SDMaxVRAM                 int     `json:"sdmaxvram"`
	SDStreamLayers            int     `json:"sdstreamlayers"`
	SDTensorTypeRules         any     `json:"sdtensortyperules"`
	SDVAEFormat               string  `json:"sdvaeformat"`
	SDLoRAModelDir            string  `json:"sdloramodeldir"`
	SDHiresUpscalersDir       string  `json:"sdhiresupscalersdir"`
	SDDiffusionFlashAttention bool    `json:"sddiffusionflashattention"`
	SDDiffusionConvDirect     bool    `json:"sddiffusionconvdirect"`
	SDVAEConvDirect           bool    `json:"sdvaeconvdirect"`
	SDUpscaler                string  `json:"sdupscaler"`
	SDVAE                     string  `json:"sdvae"`
	SDVAEAuto                 bool    `json:"sdvaeauto"`
	SDT5XXL                   string  `json:"sdt5xxl"`
	SDClip1                   string  `json:"sdclip1"`
	SDClip2                   string  `json:"sdclip2"`
	SDClipL                   string  `json:"sdclipl"`
	SDClipG                   string  `json:"sdclipg"`
	SDLoRA                    any     `json:"sdlora"`
	SDQuant                   int     `json:"sdquant"`
	SDTiledVAE                int     `json:"sdtiledvae"`
	SDFlashAttention          bool    `json:"sdflashattention"`
	SDOffloadCPU              bool    `json:"sdoffloadcpu"`
	SDVAECPU                  bool    `json:"sdvaecpu"`
	SDClipGPU                 bool    `json:"sdclipgpu"`
	SDThreads                 int     `json:"sdthreads"`
	ContextSize               int     `json:"contextsize"`
	EmbeddingsModel           string  `json:"embeddingsmodel"`
	EmbeddingsMaxCtx          int     `json:"embeddingsmaxctx"`
	EmbeddingsGPU             bool    `json:"embeddingsgpu"`
	MMProj                    any     `json:"mmproj"`
	MMProjCPU                 bool    `json:"mmprojcpu"`
	VisionMaxRes              int     `json:"visionmaxres"`
	VisionMinTokens           int     `json:"visionmintokens"`
	VisionMaxTokens           int     `json:"visionmaxtokens"`
	WhisperModel              string  `json:"whispermodel"`
	TTSModel                  string  `json:"ttsmodel"`
	TTSWAVTokenizer           string  `json:"ttswavtokenizer"`
	TalkerModel               string  `json:"talkermodel"`
	Code2WAVModel             string  `json:"code2wavmodel"`
	TTSDir                    string  `json:"ttsdir"`
	TTSGPU                    bool    `json:"ttsgpu"`
	TTSMaxLen                 int     `json:"ttsmaxlen"`
	TTSThreads                int     `json:"ttsthreads"`
	MusicLLM                  string  `json:"musicllm"`
	MusicEmbeddings           string  `json:"musicembeddings"`
	MusicDiffusion            string  `json:"musicdiffusion"`
	MusicVAE                  string  `json:"musicvae"`
	MusicLowVRAM              bool    `json:"musiclowvram"`
}

type configMetadata = RuntimeConfig

type Capabilities struct {
	LLM        bool                  `json:"llm"`
	Image      *ImageCapabilities    `json:"image,omitempty"`
	Embeddings *EmbeddingCapability  `json:"embeddings,omitempty"`
	Multimodal *MultimodalCapability `json:"multimodal,omitempty"`
	Voice      *VoiceCapabilities    `json:"voice,omitempty"`
	Music      *MusicCapabilities    `json:"music,omitempty"`
	Context    int                   `json:"context,omitempty"`
}

type ImageCapabilities struct {
	Model          string   `json:"model,omitempty"`
	Upscaler       string   `json:"upscaler,omitempty"`
	VAE            string   `json:"vae,omitempty"`
	VAEAuto        bool     `json:"vae_auto,omitempty"`
	T5XXL          string   `json:"t5xxl,omitempty"`
	Clip1          string   `json:"clip1,omitempty"`
	Clip2          string   `json:"clip2,omitempty"`
	ClipL          string   `json:"clip_l,omitempty"`
	ClipG          string   `json:"clip_g,omitempty"`
	LoRA           []string `json:"lora,omitempty"`
	Quant          int      `json:"quant,omitempty"`
	TiledVAE       int      `json:"tiled_vae,omitempty"`
	FlashAttention bool     `json:"flash_attention,omitempty"`
	OffloadCPU     bool     `json:"offload_cpu,omitempty"`
	VAECPU         bool     `json:"vae_cpu,omitempty"`
	ClipGPU        bool     `json:"clip_gpu,omitempty"`
}

type EmbeddingCapability struct {
	Model  string `json:"model,omitempty"`
	MaxCtx int    `json:"max_ctx,omitempty"`
	GPU    bool   `json:"gpu,omitempty"`
}

type MultimodalCapability struct {
	Projector    string `json:"projector,omitempty"`
	VisionMaxRes int    `json:"vision_max_res,omitempty"`
	MinTokens    int    `json:"min_tokens,omitempty"`
	MaxTokens    int    `json:"max_tokens,omitempty"`
}

type VoiceCapabilities struct {
	WhisperModel  string `json:"whisper_model,omitempty"`
	TTSModel      string `json:"tts_model,omitempty"`
	WAVTokenizer  string `json:"wav_tokenizer,omitempty"`
	TalkerModel   string `json:"talker_model,omitempty"`
	Code2WAVModel string `json:"code2wav_model,omitempty"`
	Directory     string `json:"directory,omitempty"`
	GPU           bool   `json:"gpu,omitempty"`
	MaxLen        int    `json:"max_len,omitempty"`
	Threads       int    `json:"threads,omitempty"`
}

type MusicCapabilities struct {
	LLM        string `json:"llm,omitempty"`
	Embeddings string `json:"embeddings,omitempty"`
	Diffusion  string `json:"diffusion,omitempty"`
	VAE        string `json:"vae,omitempty"`
	LowVRAM    bool   `json:"low_vram,omitempty"`
}

func LoadRuntimeConfig(path string) (RuntimeConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return RuntimeConfig{}, err
	}
	var metadata RuntimeConfig
	if err := json.Unmarshal(content, &metadata); err != nil {
		return RuntimeConfig{}, err
	}
	return metadata, nil
}

func (metadata RuntimeConfig) TextModelPath() string {
	if strings.TrimSpace(metadata.ModelParam) != "" {
		return strings.TrimSpace(metadata.ModelParam)
	}
	if value := firstStringValue(metadata.Model); value != "" {
		return value
	}
	if strings.TrimSpace(metadata.TalkerModel) != "" {
		return strings.TrimSpace(metadata.TalkerModel)
	}
	if strings.TrimSpace(metadata.WhisperModel) != "" {
		return strings.TrimSpace(metadata.WhisperModel)
	}
	return strings.TrimSpace(metadata.EmbeddingsModel)
}

func (metadata RuntimeConfig) ImageModelPath() string {
	if strings.TrimSpace(metadata.SDModel) != "" {
		return strings.TrimSpace(metadata.SDModel)
	}
	return strings.TrimSpace(metadata.SDDiffusionModel)
}

func (metadata RuntimeConfig) MMProjPath() string {
	return firstStringValue(metadata.MMProj)
}

func (metadata RuntimeConfig) TensorSplitValue() string {
	values := runtimeStringValues(metadata.TensorSplit)
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, ",")
}

func capabilitiesFromMetadata(metadata configMetadata, hasLLM bool, hasImage bool, hasEmbeddings bool, hasMultimodal bool, hasVoice bool, hasMusic bool) Capabilities {
	capabilities := Capabilities{
		LLM:     hasLLM,
		Context: metadata.ContextSize,
	}
	if hasImage {
		capabilities.Image = &ImageCapabilities{
			Model:          metadata.ImageModelPath(),
			Upscaler:       strings.TrimSpace(metadata.SDUpscaler),
			VAE:            strings.TrimSpace(metadata.SDVAE),
			VAEAuto:        metadata.SDVAEAuto,
			T5XXL:          strings.TrimSpace(metadata.SDT5XXL),
			Clip1:          strings.TrimSpace(metadata.SDClip1),
			Clip2:          strings.TrimSpace(metadata.SDClip2),
			ClipL:          strings.TrimSpace(metadata.SDClipL),
			ClipG:          strings.TrimSpace(metadata.SDClipG),
			LoRA:           stringValues(metadata.SDLoRA),
			Quant:          metadata.SDQuant,
			TiledVAE:       metadata.SDTiledVAE,
			FlashAttention: metadata.SDFlashAttention,
			OffloadCPU:     metadata.SDOffloadCPU,
			VAECPU:         metadata.SDVAECPU,
			ClipGPU:        metadata.SDClipGPU,
		}
	}
	if hasEmbeddings {
		capabilities.Embeddings = &EmbeddingCapability{
			Model:  strings.TrimSpace(metadata.EmbeddingsModel),
			MaxCtx: metadata.EmbeddingsMaxCtx,
			GPU:    metadata.EmbeddingsGPU,
		}
	}
	if hasMultimodal {
		capabilities.Multimodal = &MultimodalCapability{
			Projector:    firstStringValue(metadata.MMProj),
			VisionMaxRes: metadata.VisionMaxRes,
			MinTokens:    metadata.VisionMinTokens,
			MaxTokens:    metadata.VisionMaxTokens,
		}
	}
	if hasVoice {
		capabilities.Voice = &VoiceCapabilities{
			WhisperModel:  strings.TrimSpace(metadata.WhisperModel),
			TTSModel:      strings.TrimSpace(metadata.TTSModel),
			WAVTokenizer:  strings.TrimSpace(metadata.TTSWAVTokenizer),
			TalkerModel:   strings.TrimSpace(metadata.TalkerModel),
			Code2WAVModel: strings.TrimSpace(metadata.Code2WAVModel),
			Directory:     strings.TrimSpace(metadata.TTSDir),
			GPU:           metadata.TTSGPU,
			MaxLen:        metadata.TTSMaxLen,
			Threads:       metadata.TTSThreads,
		}
	}
	if hasMusic {
		capabilities.Music = &MusicCapabilities{
			LLM:        strings.TrimSpace(metadata.MusicLLM),
			Embeddings: strings.TrimSpace(metadata.MusicEmbeddings),
			Diffusion:  strings.TrimSpace(metadata.MusicDiffusion),
			VAE:        strings.TrimSpace(metadata.MusicVAE),
			LowVRAM:    metadata.MusicLowVRAM,
		}
	}
	return capabilities
}

func runtimeStringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, runtimeStringValues(item)...)
		}
		return values
	case float64:
		return []string{strconv.FormatFloat(typed, 'f', -1, 64)}
	case int:
		return []string{strconv.Itoa(typed)}
	default:
		return nil
	}
}
