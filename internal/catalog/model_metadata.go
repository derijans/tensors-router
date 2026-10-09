package catalog

import (
	"encoding/json"
	"os"
	"strings"

	"tensors-router/internal/modelassets"
)

func (catalog *Catalog) withMetadata(model Model, includeModelHash bool) Model {
	model.AssetState = "ready"
	model.HasLLM = true
	content, err := os.ReadFile(model.Path)
	if err != nil {
		model.Capabilities = capabilitiesFromMetadata(configMetadata{}, model.HasLLM, false, false, false, false, false)
		return model
	}
	var options map[string]json.RawMessage
	if err := json.Unmarshal(content, &options); err != nil {
		model.Capabilities = capabilitiesFromMetadata(configMetadata{}, model.HasLLM, false, false, false, false, false)
		return model
	}
	model.Options = options
	applyAssetState(&model, content)
	model.ChatTemplate = ChatTemplateProfileForConfig(content)
	var metadata configMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		model.Capabilities = capabilitiesFromMetadata(configMetadata{}, model.HasLLM, false, false, false, false, false)
		return model
	}

	applyComponentPresence(&model, metadata, options)
	model.BackendMode = strings.TrimSpace(metadata.BackendMode)
	if model.BackendMode == "vllm" {
		applyVLLMMetadata(&model, metadata)
	}
	model.MCPEnabled = metadata.MCPEnabled
	if model.HasImage {
		applyImageIdentity(&model, metadata, options)
	}
	model.Capabilities = capabilitiesFromMetadata(metadata, model.HasLLM, model.HasImage, model.HasEmbeddings, model.HasMultimodal, model.HasVoice, model.HasMusic)
	model.ConfigHash = ConfigHash(content)
	model.ModelHash = catalog.modelHash(content, includeModelHash)
	return model
}

var imageModelFields = []string{"sdmodel", "sddiffusionmodel", "sdhighnoisediffusionmodel", "sdunconddiffusionmodel"}

func applyAssetState(model *Model, content []byte) {
	unresolved, err := modelassets.UnresolvedFields(content)
	if err != nil {
		model.AssetState = "failed"
		model.AssetFailure = "invalid portable metadata"
		return
	}
	if unresolved > 0 {
		model.AssetState = "unresolved"
		model.UnresolvedFields = unresolved
	}
}

func applyComponentPresence(model *Model, metadata configMetadata, options map[string]json.RawMessage) {
	model.HasImage = metadata.ImageModelPath() != "" || portableModelField(options, imageModelFields...)
	model.HasEmbeddings = strings.TrimSpace(metadata.EmbeddingsModel) != "" || portableModelField(options, "embeddingsmodel")
	model.HasMultimodal = modelHasValue(metadata.MMProj) || portableModelField(options, "mmproj")
	model.HasVoice = hasVoiceModel(metadata) || portableModelField(options, "whispermodel", "whispercpp_vad_model", "ttsmodel", "ttswavtokenizer", "talkermodel", "code2wavmodel")
	model.HasMusic = hasMusicModel(metadata) || portableModelField(options, "musicllm", "musicembeddings", "musicdiffusion", "musicvae")
	model.HasLLM = hasLLMModel(metadata) || portableModelField(options, "model", "model_param", "draftmodel")
	if model.HasLLM {
		model.Size = regularFileSize(metadata.TextModelPath())
	}
}

func applyImageIdentity(model *Model, metadata configMetadata, options map[string]json.RawMessage) {
	model.ImageModelPath = metadata.ImageModelPath()
	if model.ImageModelPath == "" {
		model.ImageModelPath = portableModelFilename(options, imageModelFields...)
	}
	model.ImageModelName = filenameStem(model.ImageModelPath)
	model.ImageID = model.ID + "-" + model.ImageModelName
}

func (catalog *Catalog) modelHash(content []byte, includeModelHash bool) string {
	if catalog.hashStore != nil && includeModelHash {
		return catalog.hashStore.ModelHash(content)
	}
	return ModelReferenceHash(content, nil)
}

func applyVLLMMetadata(model *Model, metadata configMetadata) {
	model.HasLLM = false
	model.HasEmbeddings = false
	model.HasMultimodal = false
	model.HasVoice = false
	model.VLLMTask = strings.ToLower(strings.TrimSpace(metadata.VLLM.Task))
	if model.VLLMTask == "" || model.VLLMTask == "auto" {
		model.VLLMTask = strings.ToLower(strings.TrimSpace(metadata.VLLM.Runner))
	}
	if model.VLLMTask == "" || model.VLLMTask == "auto" {
		model.VLLMTask = "generate"
	}
	servedNames := append([]string{}, metadata.VLLM.ServedNames...)
	for _, adapter := range metadata.VLLM.StaticAdapters {
		servedNames = append(servedNames, adapter.Name)
	}
	model.ServedNames = normalizedServedNames(servedNames)
	switch model.VLLMTask {
	case "generate", "generation", "text", "chat", "multimodal", "generative_scoring":
		model.HasLLM = true
		model.HasMultimodal = model.VLLMTask == "multimodal"
	case "embed", "embedding", "embeddings", "classify", "classification", "score", "scoring", "reward", "rerank", "pooling":
		model.HasEmbeddings = true
	case "speech", "transcription", "translation", "realtime":
		model.HasVoice = true
	}
	model.Size = 0
}

func normalizedServedNames(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func regularFileSize(path string) int64 {
	info, err := os.Stat(strings.TrimSpace(path))
	if err != nil || !info.Mode().IsRegular() {
		return 0
	}
	return info.Size()
}

func portableModelField(options map[string]json.RawMessage, fields ...string) bool {
	for _, field := range fields {
		if value, found := options[field+"_hash"]; found && len(value) > 0 && string(value) != "null" {
			return true
		}
	}
	return false
}

func portableModelFilename(options map[string]json.RawMessage, fields ...string) string {
	for _, field := range fields {
		value, found := options[field+"_filename"]
		if !found {
			continue
		}
		var scalar string
		if json.Unmarshal(value, &scalar) == nil && scalar != "" {
			return scalar
		}
		var array []string
		if json.Unmarshal(value, &array) == nil && len(array) > 0 {
			return array[0]
		}
	}
	return ""
}

func hasLLMModel(metadata configMetadata) bool {
	if strings.TrimSpace(metadata.ModelParam) != "" {
		return true
	}
	if modelHasValue(metadata.Model) {
		return true
	}
	return !metadata.NoModel && strings.TrimSpace(metadata.SDModel) == ""
}

func hasVoiceModel(metadata configMetadata) bool {
	return strings.TrimSpace(metadata.WhisperModel) != "" ||
		strings.TrimSpace(metadata.TTSModel) != "" ||
		strings.TrimSpace(metadata.TTSWAVTokenizer) != "" ||
		strings.TrimSpace(metadata.TalkerModel) != "" ||
		strings.TrimSpace(metadata.Code2WAVModel) != "" ||
		strings.TrimSpace(metadata.TTSDir) != ""
}

func hasMusicModel(metadata configMetadata) bool {
	return strings.TrimSpace(metadata.MusicLLM) != "" ||
		strings.TrimSpace(metadata.MusicEmbeddings) != "" ||
		strings.TrimSpace(metadata.MusicDiffusion) != "" ||
		strings.TrimSpace(metadata.MusicVAE) != ""
}

func modelHasValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		for _, item := range typed {
			if modelHasValue(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
