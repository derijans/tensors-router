package cluster

import "tensors-router/internal/catalog"

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

type Model struct {
	PublicID      string               `json:"public_id"`
	LocalID       string               `json:"local_id"`
	ImageID       string               `json:"image_id,omitempty"`
	PublicImageID string               `json:"public_image_id,omitempty"`
	Filename      string               `json:"filename"`
	Created       int64                `json:"created"`
	HasLLM        bool                 `json:"has_llm"`
	HasImage      bool                 `json:"has_image"`
	HasEmbeddings bool                 `json:"has_embeddings"`
	HasMultimodal bool                 `json:"has_multimodal"`
	ModelHash     string               `json:"model_hash"`
	ConfigHash    string               `json:"config_hash"`
	Capabilities  catalog.Capabilities `json:"capabilities"`
	Source        string               `json:"source"`
	NodeID        string               `json:"node_id"`
	NodeURL       string               `json:"node_url,omitempty"`
	Available     bool                 `json:"available"`
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
}

func LocalModels(models []catalog.Model, nodeID string, nodeURL string, source string) []Model {
	records := make([]Model, 0, len(models))
	for _, model := range models {
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
			ModelHash:     model.ModelHash,
			ConfigHash:    model.ConfigHash,
			Capabilities:  model.Capabilities,
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
