package cluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"tensors-router/internal/backendmode"
	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/catalog"
)

const ErrorCodeDuplicateNode = "duplicate_node"

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (clusterError *Error) Error() string {
	return clusterError.Message
}

func NewDuplicateNodeError(message string) error {
	return &Error{Code: ErrorCodeDuplicateNode, Message: message}
}

func ErrorCode(err error) string {
	var clusterError *Error
	if errors.As(err, &clusterError) {
		return clusterError.Code
	}
	return ""
}

func DuplicateNodeError(nodeID string, nodeURL string) error {
	return NewDuplicateNodeError(fmt.Sprintf("node identity conflicts with an existing owner: node_id=%q node_url=%q", nodeID, nodeURL))
}

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
	BackendModeVLLM       = backendmode.VLLM
)

const (
	RouteLaneText  = "text"
	RouteLaneImage = "image"
	RouteLaneVoice = "voice"
	RouteLaneMusic = "music"
)

type Model struct {
	PublicID         string                          `json:"public_id"`
	LocalID          string                          `json:"local_id"`
	ImageID          string                          `json:"image_id,omitempty"`
	PublicImageID    string                          `json:"public_image_id,omitempty"`
	Filename         string                          `json:"filename"`
	Created          int64                           `json:"created"`
	Size             int64                           `json:"size,omitempty"`
	HasLLM           bool                            `json:"has_llm"`
	HasImage         bool                            `json:"has_image"`
	HasEmbeddings    bool                            `json:"has_embeddings"`
	HasMultimodal    bool                            `json:"has_multimodal"`
	HasVoice         bool                            `json:"has_voice"`
	HasMusic         bool                            `json:"has_music"`
	MCPEnabled       bool                            `json:"mcp_enabled,omitempty"`
	ModelHash        string                          `json:"model_hash"`
	ConfigHash       string                          `json:"config_hash"`
	Capabilities     catalog.Capabilities            `json:"capabilities"`
	Options          map[string]json.RawMessage      `json:"options,omitempty"`
	BackendMode      string                          `json:"backend_mode"`
	Source           string                          `json:"source"`
	NodeID           string                          `json:"node_id"`
	NodeURL          string                          `json:"node_url,omitempty"`
	Available        bool                            `json:"available"`
	Loaded           bool                            `json:"loaded,omitempty"`
	EmbeddingsLoaded bool                            `json:"embeddings_loaded,omitempty"`
	AssetState       string                          `json:"asset_state,omitempty"`
	UnresolvedFields int                             `json:"unresolved_fields,omitempty"`
	AssetFailure     string                          `json:"asset_failure,omitempty"`
	Benchmark        *routerbenchmark.ModelBenchmark `json:"benchmark,omitempty"`
	Disabled         bool                            `json:"disabled,omitempty"`
	ServedNames      []string                        `json:"served_names,omitempty"`
	VLLMTask         string                          `json:"vllm_task,omitempty"`
}

type Snapshot struct {
	NodeID  string  `json:"node_id"`
	NodeURL string  `json:"node_url"`
	Models  []Model `json:"models"`
}

type ModelStateRequest struct {
	NodeID  string `json:"node_id"`
	LocalID string `json:"local_id"`
	Enabled bool   `json:"enabled"`
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
	primaryIDs := make(map[string]struct{}, len(models))
	servedNameOwners := make(map[string]int)
	for _, model := range models {
		primaryIDs[model.ID] = struct{}{}
		seen := make(map[string]struct{}, len(model.ServedNames))
		for _, servedName := range model.ServedNames {
			servedName = strings.TrimSpace(servedName)
			if servedName == "" || servedName == model.ID {
				continue
			}
			if _, duplicate := seen[servedName]; duplicate {
				continue
			}
			seen[servedName] = struct{}{}
			servedNameOwners[servedName]++
		}
	}
	records := make([]Model, 0, len(models))
	for _, model := range models {
		modelBackendMode := backendmode.Normalize(model.BackendMode)
		if modelBackendMode == "" {
			modelBackendMode = fallbackMode
		}
		record := Model{
			PublicID:         model.ID,
			LocalID:          model.ID,
			ImageID:          model.ImageID,
			PublicImageID:    model.ImageID,
			Filename:         model.Filename,
			Created:          model.Created,
			Size:             model.Size,
			HasLLM:           model.HasLLM,
			HasImage:         model.HasImage,
			HasEmbeddings:    model.HasEmbeddings,
			HasMultimodal:    model.HasMultimodal,
			HasVoice:         model.HasVoice,
			HasMusic:         model.HasMusic,
			MCPEnabled:       model.MCPEnabled,
			ModelHash:        model.ModelHash,
			ConfigHash:       model.ConfigHash,
			Capabilities:     model.Capabilities,
			Options:          catalog.SanitizedOptions(model.Options),
			BackendMode:      modelBackendMode,
			Source:           source,
			NodeID:           nodeID,
			NodeURL:          nodeURL,
			Available:        true,
			AssetState:       model.AssetState,
			UnresolvedFields: model.UnresolvedFields,
			AssetFailure:     model.AssetFailure,
			ServedNames:      append([]string{}, model.ServedNames...),
			VLLMTask:         model.VLLMTask,
		}
		records = append(records, record)
		for _, servedName := range model.ServedNames {
			servedName = strings.TrimSpace(servedName)
			if servedName == "" || servedName == model.ID || servedNameOwners[servedName] != 1 {
				continue
			}
			if _, conflicts := primaryIDs[servedName]; conflicts {
				continue
			}
			alias := record
			alias.PublicID = servedName
			alias.ServedNames = append([]string{}, record.ServedNames...)
			records = append(records, alias)
		}
	}
	return records
}

func WithMCPAvailability(models []Model, available bool) []Model {
	if available {
		return models
	}
	result := make([]Model, len(models))
	copy(result, models)
	for index := range result {
		result[index].MCPEnabled = false
		result[index].Capabilities.MCP = false
	}
	return result
}

func PublicCatalogModels(models []Model) []catalog.Model {
	result := make([]catalog.Model, 0, len(models))
	seen := map[string]struct{}{}
	for _, model := range models {
		mode := backendmode.Normalize(model.BackendMode)
		if model.Disabled || !model.HasLLM && (mode != backendmode.VLLM || !model.HasEmbeddings && !model.HasVoice) {
			continue
		}
		if _, ok := seen[model.PublicID]; ok {
			continue
		}
		seen[model.PublicID] = struct{}{}
		result = append(result, catalog.Model{
			ID:            model.PublicID,
			Created:       model.Created,
			HasLLM:        model.HasLLM,
			HasEmbeddings: model.HasEmbeddings,
			HasVoice:      model.HasVoice,
			ModelHash:     model.ModelHash,
			ConfigHash:    model.ConfigHash,
			BackendMode:   mode,
			Capabilities:  model.Capabilities,
			ServedNames:   append([]string{}, model.ServedNames...),
			VLLMTask:      model.VLLMTask,
		})
	}
	return result
}
