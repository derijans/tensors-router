package cook

const (
	SourceConfig = "config"
	SourceFile   = "file"
)

const (
	KindText       = "text"
	KindImage      = "image"
	KindEmbeddings = "embeddings"
)

type Component struct {
	Kind     string `json:"kind"`
	NodeID   string `json:"node_id"`
	NodeURL  string `json:"node_url,omitempty"`
	Source   string `json:"source"`
	ModelID  string `json:"model_id,omitempty"`
	ImageID  string `json:"image_id,omitempty"`
	FilePath string `json:"file_path,omitempty"`
}

type NodeConfigRequest struct {
	ID         string      `json:"id"`
	Overwrite  bool        `json:"overwrite"`
	DryRun     bool        `json:"dry_run"`
	Components []Component `json:"components"`
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
