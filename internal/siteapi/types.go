package siteapi

import (
	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/downloader"
	"tensors-router/internal/hardware"
	"tensors-router/internal/inventory"
	"tensors-router/internal/recipes"
)

type ModelAssetConfigRequest struct {
	NodeID   string `json:"node_id,omitempty"`
	NodeURL  string `json:"node_url,omitempty"`
	ID       string `json:"id"`
	Filename string `json:"filename,omitempty"`
}

type ModelAssetConfigResponse struct {
	ID       string                  `json:"id"`
	Filename string                  `json:"filename"`
	Content  []byte                  `json:"content,omitempty"`
	Results  []ModelAssetFieldResult `json:"results,omitempty"`
}

type ModelAssetFieldResult struct {
	Field        string `json:"field"`
	Hash         string `json:"hash"`
	Resolved     bool   `json:"resolved"`
	Failure      string `json:"failure,omitempty"`
	Source       string `json:"source,omitempty"`
	Verification string `json:"verification,omitempty"`
	Commit       string `json:"commit,omitempty"`
}

type ModelAssetBindingRequest struct {
	NodeID         string `json:"node_id,omitempty"`
	NodeURL        string `json:"node_url,omitempty"`
	SHA256         string `json:"sha256"`
	Repository     string `json:"repository"`
	RepositoryPath string `json:"repository_path"`
	Commit         string `json:"commit"`
	Token          string `json:"token,omitempty"`
}

type ModelAssetCandidateRequest struct {
	NodeID   string `json:"node_id,omitempty"`
	NodeURL  string `json:"node_url,omitempty"`
	SHA256   string `json:"sha256"`
	Filename string `json:"filename"`
	Token    string `json:"token,omitempty"`
}

type ModelAssetSubstitutionRequest struct {
	NodeID         string `json:"node_id,omitempty"`
	NodeURL        string `json:"node_url,omitempty"`
	ID             string `json:"id"`
	Filename       string `json:"filename,omitempty"`
	Field          string `json:"field"`
	Position       *int   `json:"position,omitempty"`
	ExpectedSHA256 string `json:"expected_sha256"`
	SHA256         string `json:"sha256"`
	Repository     string `json:"repository"`
	RepositoryPath string `json:"repository_path"`
	Commit         string `json:"commit"`
	Token          string `json:"token,omitempty"`
	Confirm        bool   `json:"confirm"`
}

type NodeInventory struct {
	NodeID      string                 `json:"node_id"`
	NodeURL     string                 `json:"node_url,omitempty"`
	Source      string                 `json:"source"`
	Role        string                 `json:"role"`
	BackendMode string                 `json:"backend_mode"`
	Available   bool                   `json:"available"`
	Hardware    hardware.Info          `json:"hardware"`
	Models      []cluster.Model        `json:"models"`
	Files       []inventory.FileRecord `json:"files"`
	Error       string                 `json:"error,omitempty"`
}

type InventoryResponse struct {
	Role            string                  `json:"role"`
	NodeID          string                  `json:"node_id"`
	NodeURL         string                  `json:"node_url,omitempty"`
	Nodes           []NodeInventory         `json:"nodes"`
	Models          []cluster.Model         `json:"models"`
	Recipes         []recipes.Recipe        `json:"recipes"`
	OptionCatalog   []cook.OptionDefinition `json:"option_catalog"`
	ObservedOptions []cook.OptionDefinition `json:"observed_options"`
}

type CookRequest struct {
	ID         string           `json:"id"`
	Overwrite  bool             `json:"overwrite"`
	Components []cook.Component `json:"components"`
	Options    cook.Options     `json:"options,omitempty"`
}

type CookResponse struct {
	Plan       cook.Plan              `json:"plan"`
	Recipe     *recipes.Recipe        `json:"recipe,omitempty"`
	Validation []cook.ValidationIssue `json:"validation,omitempty"`
}

type ConfigFileRequest struct {
	NodeID    string       `json:"node_id,omitempty"`
	NodeURL   string       `json:"node_url,omitempty"`
	ID        string       `json:"id,omitempty"`
	Filename  string       `json:"filename,omitempty"`
	Overwrite bool         `json:"overwrite"`
	Options   cook.Options `json:"options"`
}

type ConfigFileResponse struct {
	NodeID         string       `json:"node_id"`
	NodeURL        string       `json:"node_url,omitempty"`
	ID             string       `json:"id"`
	Filename       string       `json:"filename"`
	WouldOverwrite bool         `json:"would_overwrite,omitempty"`
	Deleted        bool         `json:"deleted,omitempty"`
	Options        cook.Options `json:"options,omitempty"`
}

type RouterProcessStatus struct {
	Managed      bool   `json:"managed"`
	Running      bool   `json:"running"`
	URL          string `json:"url"`
	PID          int    `json:"pid,omitempty"`
	CanShutdown  bool   `json:"can_shutdown"`
	CanForceKill bool   `json:"can_force_kill"`
	Error        string `json:"error,omitempty"`
}

type DownloadCapability struct {
	NodeID     string                        `json:"node_id"`
	NodeURL    string                        `json:"node_url,omitempty"`
	Available  bool                          `json:"available"`
	Capability downloader.Capability         `json:"capability"`
	Devices    []downloader.DeviceCapability `json:"devices"`
}

type DownloadCapabilitiesResponse struct {
	Nodes []DownloadCapability `json:"nodes"`
}

type DownloadSearchRequest struct {
	NodeID string `json:"node_id,omitempty"`
	Token  string `json:"token,omitempty"`
	downloader.SearchRequest
}

type DownloadRepositoryRequest struct {
	NodeID string `json:"node_id,omitempty"`
	downloader.RepositoryRequest
}

type DownloadPlanRequest struct {
	NodeID string `json:"node_id,omitempty"`
	downloader.PlanRequest
}

type DownloadCreateJobRequest struct {
	NodeID string `json:"node_id,omitempty"`
	downloader.CreateJobRequest
}

type DownloadJobRequest struct {
	NodeID string `json:"node_id,omitempty"`
}

type DownloadLibraryResponse struct {
	Artifacts []downloader.ArtifactRecord `json:"artifacts"`
	Jobs      []downloader.DownloadJob    `json:"jobs"`
}

type ModelFileHashRequest struct {
	NodeID string `json:"node_id,omitempty"`
	Path   string `json:"path"`
}

type ModelFileHashResponse struct {
	NodeID string `json:"node_id"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
