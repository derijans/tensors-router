package catalog

import "strings"

type configMetadata struct {
	Model            any    `json:"model"`
	ModelParam       string `json:"model_param"`
	NoModel          bool   `json:"nomodel"`
	SDModel          string `json:"sdmodel"`
	SDUpscaler       string `json:"sdupscaler"`
	SDVAE            string `json:"sdvae"`
	SDVAEAuto        bool   `json:"sdvaeauto"`
	SDT5XXL          string `json:"sdt5xxl"`
	SDClip1          string `json:"sdclip1"`
	SDClip2          string `json:"sdclip2"`
	SDClipL          string `json:"sdclipl"`
	SDClipG          string `json:"sdclipg"`
	SDLoRA           any    `json:"sdlora"`
	SDQuant          int    `json:"sdquant"`
	SDTiledVAE       int    `json:"sdtiledvae"`
	SDFlashAttention bool   `json:"sdflashattention"`
	SDOffloadCPU     bool   `json:"sdoffloadcpu"`
	SDVAECPU         bool   `json:"sdvaecpu"`
	SDClipGPU        bool   `json:"sdclipgpu"`
	ContextSize      int    `json:"contextsize"`
	EmbeddingsModel  string `json:"embeddingsmodel"`
	EmbeddingsMaxCtx int    `json:"embeddingsmaxctx"`
	EmbeddingsGPU    bool   `json:"embeddingsgpu"`
	MMProj           any    `json:"mmproj"`
	VisionMaxRes     int    `json:"visionmaxres"`
	VisionMinTokens  int    `json:"visionmintokens"`
	VisionMaxTokens  int    `json:"visionmaxtokens"`
}

type Capabilities struct {
	LLM        bool                  `json:"llm"`
	Image      *ImageCapabilities    `json:"image,omitempty"`
	Embeddings *EmbeddingCapability  `json:"embeddings,omitempty"`
	Multimodal *MultimodalCapability `json:"multimodal,omitempty"`
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

func capabilitiesFromMetadata(metadata configMetadata, hasLLM bool, hasImage bool, hasEmbeddings bool, hasMultimodal bool) Capabilities {
	capabilities := Capabilities{
		LLM:     hasLLM,
		Context: metadata.ContextSize,
	}
	if hasImage {
		capabilities.Image = &ImageCapabilities{
			Model:          strings.TrimSpace(metadata.SDModel),
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
	return capabilities
}
