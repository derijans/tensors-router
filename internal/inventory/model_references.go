package inventory

import (
	"strings"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

type roleAsset struct {
	path string
	role string
}

func referencesByPath(models []cluster.Model) map[string][]pathReference {
	references := map[string][]pathReference{}
	for _, model := range models {
		modelName := model.PublicID
		if modelName == "" {
			modelName = model.LocalID
		}
		for _, asset := range capabilityAssets(model.Capabilities) {
			addReference(references, asset.path, asset.role, modelName)
		}
	}
	return references
}

func capabilityAssets(capabilities catalog.Capabilities) []roleAsset {
	var assets []roleAsset
	if capabilities.Embeddings != nil {
		assets = append(assets, roleAsset{capabilities.Embeddings.Model, RoleEmbeddings})
	}
	if capabilities.Multimodal != nil {
		assets = append(assets, roleAsset{capabilities.Multimodal.Projector, RoleMultimodal})
	}
	if capabilities.Image != nil {
		assets = append(assets, imageAssets(*capabilities.Image)...)
	}
	if capabilities.Voice != nil {
		voice := capabilities.Voice
		assets = append(assets, sameRoleAssets(RoleVoice, voice.WhisperModel, voice.VADModel, voice.TTSModel, voice.WAVTokenizer, voice.Directory)...)
	}
	if capabilities.Music != nil {
		music := capabilities.Music
		assets = append(assets, sameRoleAssets(RoleMusic, music.LLM, music.Embeddings, music.Diffusion, music.VAE)...)
	}
	return assets
}

func imageAssets(image catalog.ImageCapabilities) []roleAsset {
	assets := []roleAsset{{image.Model, RoleImage}, {image.VAE, RoleVAE}}
	assets = append(assets, sameRoleAssets(RoleClip, image.Clip1, image.Clip2, image.ClipL, image.ClipG)...)
	assets = append(assets, roleAsset{image.T5XXL, RoleT5}, roleAsset{image.Upscaler, RoleUpscaler})
	return append(assets, sameRoleAssets(RoleLoRA, image.LoRA...)...)
}

func sameRoleAssets(role string, paths ...string) []roleAsset {
	assets := make([]roleAsset, 0, len(paths))
	for _, path := range paths {
		assets = append(assets, roleAsset{path, role})
	}
	return assets
}

func addReference(references map[string][]pathReference, path string, role string, model string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, key := range pathKeys(path) {
		references[key] = append(references[key], pathReference{role: role, model: model})
	}
}

type pathReference struct {
	role  string
	model string
}
