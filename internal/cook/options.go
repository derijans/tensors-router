package cook

import (
	"encoding/json"
	"sort"
	"strings"
)

const (
	LaneCommon     = "common"
	LaneText       = KindText
	LaneImage      = KindImage
	LaneEmbeddings = KindEmbeddings
	LaneMultimodal = "multimodal"
)

const (
	ValueString = "string"
	ValueNumber = "number"
	ValueBool   = "bool"
	ValueJSON   = "json"
)

var optionCatalog = []OptionDefinition{
	option("model", "Model", LaneText, ValueJSON, "", false, "kobold", "llama_sdcpp"),
	option("model_param", "Model Path", LaneText, ValueString, "--model", false, "kobold", "llama_sdcpp"),
	option("nomodel", "No Model", LaneText, ValueBool, "", false, "kobold", "llama_sdcpp"),
	option("contextsize", "Context Size", LaneText, ValueNumber, "--ctx-size", false, "kobold", "llama_sdcpp"),
	option("threads", "Threads", LaneText, ValueNumber, "--threads", false, "kobold", "llama_sdcpp"),
	option("batchsize", "Batch Size", LaneText, ValueNumber, "--batch-size", false, "kobold", "llama_sdcpp"),
	option("gpulayers", "GPU Layers", LaneText, ValueNumber, "--n-gpu-layers", false, "kobold", "llama_sdcpp"),
	option("splitmode", "Split Mode", LaneText, ValueString, "--split-mode", false, "kobold", "llama_sdcpp"),
	option("tensor_split", "Tensor Split", LaneText, ValueJSON, "--tensor-split", false, "kobold", "llama_sdcpp"),
	option("maingpu", "Main GPU", LaneText, ValueNumber, "--main-gpu", false, "kobold", "llama_sdcpp"),
	option("usemmap", "Use MMap", LaneText, ValueBool, "--no-mmap", false, "kobold", "llama_sdcpp"),
	option("usemlock", "Use MLock", LaneText, ValueBool, "--mlock", false, "kobold", "llama_sdcpp"),
	option("quantkv", "KV Cache Quant", LaneText, ValueString, "--cache-type-k", false, "kobold", "llama_sdcpp"),
	option("usecuda", "Use CUDA", LaneText, ValueBool, "", true, "kobold"),
	option("usecublas", "Use cuBLAS", LaneText, ValueBool, "", true, "kobold"),
	option("flashattention", "Flash Attention", LaneText, ValueBool, "", false, "kobold"),
	option("lowvram", "Low VRAM", LaneText, ValueBool, "", false, "kobold"),
	option("blasbatchsize", "BLAS Batch Size", LaneText, ValueNumber, "", false, "kobold"),
	option("blasthreads", "BLAS Threads", LaneText, ValueNumber, "", false, "kobold"),
	option("ropeconfig", "RoPE Config", LaneText, ValueJSON, "", false, "kobold"),
	option("mmproj", "Multimodal Projector", LaneMultimodal, ValueJSON, "--mmproj", false, "kobold", "llama_sdcpp"),
	option("mmprojcpu", "Projector On CPU", LaneMultimodal, ValueBool, "--no-mmproj-offload", false, "kobold", "llama_sdcpp"),
	option("visionmaxres", "Vision Max Resolution", LaneMultimodal, ValueNumber, "", false, "kobold", "llama_sdcpp"),
	option("visionmintokens", "Vision Min Tokens", LaneMultimodal, ValueNumber, "--image-min-tokens", false, "kobold", "llama_sdcpp"),
	option("visionmaxtokens", "Vision Max Tokens", LaneMultimodal, ValueNumber, "--image-max-tokens", false, "kobold", "llama_sdcpp"),
	option("embeddingsmodel", "Embeddings Model", LaneEmbeddings, ValueString, "--model", false, "kobold", "llama_sdcpp"),
	option("embeddingsmaxctx", "Embeddings Max Context", LaneEmbeddings, ValueNumber, "", false, "kobold", "llama_sdcpp"),
	option("embeddingsgpu", "Embeddings GPU", LaneEmbeddings, ValueBool, "", false, "kobold", "llama_sdcpp"),
	option("sdmodel", "Image Model", LaneImage, ValueString, "--model", false, "kobold", "llama_sdcpp"),
	option("sdupscaler", "Upscaler", LaneImage, ValueString, "--upscale-model", false, "kobold", "llama_sdcpp"),
	option("sdvae", "VAE", LaneImage, ValueString, "--vae", false, "kobold", "llama_sdcpp"),
	option("sdvaeauto", "Auto VAE", LaneImage, ValueBool, "", false, "kobold"),
	option("sdt5xxl", "T5 XXL", LaneImage, ValueString, "--t5xxl", false, "kobold", "llama_sdcpp"),
	option("sdclip1", "CLIP 1", LaneImage, ValueString, "--clip_l", false, "kobold", "llama_sdcpp"),
	option("sdclip2", "CLIP 2", LaneImage, ValueString, "--clip_g", false, "kobold", "llama_sdcpp"),
	option("sdclipl", "CLIP L", LaneImage, ValueString, "--clip_l", false, "kobold", "llama_sdcpp"),
	option("sdclipg", "CLIP G", LaneImage, ValueString, "--clip_g", false, "kobold", "llama_sdcpp"),
	option("sdlora", "LoRA", LaneImage, ValueJSON, "", false, "kobold"),
	option("sdquant", "Image Quant", LaneImage, ValueNumber, "", false, "kobold"),
	option("sdtiledvae", "Tiled VAE", LaneImage, ValueNumber, "--vae-tiling", false, "kobold", "llama_sdcpp"),
	option("sdflashattention", "Image Flash Attention", LaneImage, ValueBool, "--fa", false, "kobold", "llama_sdcpp"),
	option("sdoffloadcpu", "Offload To CPU", LaneImage, ValueBool, "--offload-to-cpu", false, "kobold", "llama_sdcpp"),
	option("sdvaecpu", "VAE On CPU", LaneImage, ValueBool, "--vae-on-cpu", false, "kobold", "llama_sdcpp"),
	option("sdclipgpu", "CLIP GPU", LaneImage, ValueBool, "", false, "kobold"),
	option("sdthreads", "Image Threads", LaneImage, ValueNumber, "--threads", false, "kobold", "llama_sdcpp"),
}

var catalogByKey = buildOptionByKey()

func option(key string, name string, lane string, valueType string, nativeFlag string, cudaOnly bool, backends ...string) OptionDefinition {
	return OptionDefinition{
		Key:        key,
		Name:       name,
		Lane:       lane,
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
		if !ok || definition.Lane == LaneCommon || allowed[definition.Lane] {
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
