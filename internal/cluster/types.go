package cluster

import (
	"encoding/json"

	"tensors-router/internal/backendmode"
	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/catalog"
)

const (
	RoleStandalone = "standalone"
	RoleMaster     = "master"
	RoleSlave      = "slave"
)

const (
	SourceLocal  = "local"
	SourceMaster = "master"
	SourceSlave  = "slave"
)

const (
	BackendModeKobold     = backendmode.Kobold
	BackendModeLlamaSDCPP = backendmode.LlamaSDCPP
)

const (
	RouteLaneText  = "text"
	RouteLaneImage = "image"
	RouteLaneVoice = "voice"
	RouteLaneMusic = "music"
)

type Model struct {
	PublicID      string                          `json:"public_id"`
	LocalID       string                          `json:"local_id"`
	ImageID       string                          `json:"image_id,omitempty"`
	PublicImageID string                          `json:"public_image_id,omitempty"`
	Filename      string                          `json:"filename"`
	Created       int64                           `json:"created"`
	HasLLM        bool                            `json:"has_llm"`
	HasImage      bool                            `json:"has_image"`
	HasEmbeddings bool                            `json:"has_embeddings"`
	HasMultimodal bool                            `json:"has_multimodal"`
	HasVoice      bool                            `json:"has_voice"`
	HasMusic      bool                            `json:"has_music"`
	ModelHash     string                          `json:"model_hash"`
	ConfigHash    string                          `json:"config_hash"`
	Capabilities  catalog.Capabilities            `json:"capabilities"`
	Options       map[string]json.RawMessage      `json:"options,omitempty"`
	BackendMode   string                          `json:"backend_mode"`
	Source        string                          `json:"source"`
	NodeID        string                          `json:"node_id"`
	NodeURL       string                          `json:"node_url,omitempty"`
	Available     bool                            `json:"available"`
	Benchmark     *routerbenchmark.ModelBenchmark `json:"benchmark,omitempty"`
}

type Snapshot struct {
	NodeID  string  `json:"node_id"`
	NodeURL string  `json:"node_url"`
	Models  []Model `json:"models"`
}

type Route struct {
	PublicID      string
	LocalID       string
	PublicImageID string
	LocalImageID  string
	Filename      string
	NodeID        string
	NodeURL       string
	Remote        bool
	Lane          string
	BackendMode   string
}

func LocalModels(models []catalog.Model, nodeID string, nodeURL string, source string) []Model {
	return LocalModelsWithBackendMode(models, nodeID, nodeURL, source, BackendModeKobold)
}

func LocalModelsWithBackendMode(models []catalog.Model, nodeID string, nodeURL string, source string, backendMode string) []Model {
	fallbackMode, err := backendmode.Resolve("", backendMode)
	if err != nil {
		fallbackMode = BackendModeKobold
	}
	records := make([]Model, 0, len(models))
	for _, model := range models {
		modelBackendMode := backendmode.Normalize(model.BackendMode)
		if modelBackendMode == "" {
			modelBackendMode = fallbackMode
		}
		records = append(records, Model{
			PublicID:      model.ID,
			LocalID:       model.ID,
			ImageID:       model.ImageID,
			PublicImageID: model.ImageID,
			Filename:      model.Filename,
			Created:       model.Created,
			HasLLM:        model.HasLLM,
			HasImage:      model.HasImage,
			HasEmbeddings: model.HasEmbeddings,
			HasMultimodal: model.HasMultimodal,
			HasVoice:      model.HasVoice,
			HasMusic:      model.HasMusic,
			ModelHash:     model.ModelHash,
			ConfigHash:    model.ConfigHash,
			Capabilities:  model.Capabilities,
			Options:       model.Options,
			BackendMode:   modelBackendMode,
			Source:        source,
			NodeID:        nodeID,
			NodeURL:       nodeURL,
			Available:     true,
		})
	}
	return records
}

func PublicCatalogModels(models []Model) []catalog.Model {
	result := make([]catalog.Model, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		if !model.HasLLM {
			continue
		}
		if _, ok := seen[model.PublicID]; ok {
			continue
		}
		seen[model.PublicID] = struct{}{}
		result = append(result, catalog.Model{
			ID:           model.PublicID,
			Created:      model.Created,
			HasLLM:       model.HasLLM,
			ModelHash:    model.ModelHash,
			ConfigHash:   model.ConfigHash,
			Capabilities: model.Capabilities,
		})
	}
	return result
}
