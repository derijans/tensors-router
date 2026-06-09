package cook

import "encoding/json"

const (
	SourceConfig = "config"
	SourceFile   = "file"
)

const (
	KindText       = "text"
	KindImage      = "image"
	KindEmbeddings = "embeddings"
	KindVoice      = "voice"
	KindMusic      = "music"
)

type Component struct {
	Kind      string `json:"kind"`
	NodeID    string `json:"node_id"`
	NodeURL   string `json:"node_url,omitempty"`
	Source    string `json:"source"`
	ModelID   string `json:"model_id,omitempty"`
	ImageID   string `json:"image_id,omitempty"`
	FilePath  string `json:"file_path,omitempty"`
	OptionKey string `json:"option_key,omitempty"`
}

type NodeConfigRequest struct {
	ID         string      `json:"id"`
	Overwrite  bool        `json:"overwrite"`
	DryRun     bool        `json:"dry_run"`
	Components []Component `json:"components"`
	Options    Options     `json:"options,omitempty"`
}

type ConfigResult struct {
	NodeID         string   `json:"node_id"`
	NodeURL        string   `json:"node_url,omitempty"`
	ModelID        string   `json:"model_id"`
	ImageID        string   `json:"image_id,omitempty"`
	Filename       string   `json:"filename"`
	Kinds          []string `json:"kinds"`
	Reused         bool     `json:"reused"`
	WouldOverwrite bool     `json:"would_overwrite,omitempty"`
}

type Plan struct {
	ID                   string         `json:"id"`
	PublicID             string         `json:"public_id"`
	PublicImageID        string         `json:"public_image_id,omitempty"`
	RequiresMasterRecipe bool           `json:"requires_master_recipe"`
	Configs              []ConfigResult `json:"configs"`
}

type Options map[string]json.RawMessage

type ValidationIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	NodeID   string `json:"node_id,omitempty"`
	Field    string `json:"field,omitempty"`
}

type OptionDefinition struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Lane       string   `json:"lane"`
	Section    string   `json:"section,omitempty"`
	ValueType  string   `json:"value_type"`
	Choices    []string `json:"choices,omitempty"`
	Backends   []string `json:"backends"`
	NativeFlag string   `json:"native_flag,omitempty"`
	CUDAOnly   bool     `json:"cuda_only,omitempty"`
	ModelRole  string   `json:"model_role,omitempty"`
	Default    string   `json:"default,omitempty"`
	Source     string   `json:"source,omitempty"`
	Known      bool     `json:"known"`
}
