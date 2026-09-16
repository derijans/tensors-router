package siteapi

import (
	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/downloader"
	"tensors-router/internal/hardware"
	"tensors-router/internal/inventory"
	"tensors-router/internal/recipes"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/vllm"
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

type NodeState struct {
	NodeID          string             `json:"node_id"`
	Backends        []NodeStateBackend `json:"backends"`
	ActiveRequests  []string           `json:"active_requests"`
	FFmpegAvailable bool               `json:"ffmpeg_available"`
	FFmpegPath      string             `json:"ffmpeg_path,omitempty"`
}

type NodeStateBackend struct {
	ID                       string              `json:"id"`
	DisplayName              string              `json:"display_name"`
	Mode                     string              `json:"mode"`
	LifecycleState           string              `json:"lifecycle_state,omitempty"`
	SelectedProfile          string              `json:"selected_profile,omitempty"`
	DetectedProfile          string              `json:"detected_profile,omitempty"`
	RuntimeVersion           string              `json:"runtime_version,omitempty"`
	InitializationJobID      string              `json:"initialization_job_id,omitempty"`
	InitializationPhase      string              `json:"initialization_phase,omitempty"`
	InitializationBytes      int64               `json:"initialization_bytes,omitempty"`
	InitializationTotalBytes int64               `json:"initialization_total_bytes,omitempty"`
	ManifestTrust            string              `json:"manifest_trust,omitempty"`
	Error                    string              `json:"error,omitempty"`
	Retryable                bool                `json:"retryable,omitempty"`
	LoadedModels             []NodeStateModelRow `json:"loaded_models"`
}

type NodeStateModelRow struct {
	ModelID    string `json:"model_id"`
	Lane       string `json:"lane"`
	RuntimeID  string `json:"runtime_id"`
	Generation uint64 `json:"generation"`
}

type NodeUnloadRequest struct {
	NodeID             string `json:"node_id"`
	BackendID          string `json:"backend_id"`
	RuntimeID          string `json:"runtime_id"`
	ExpectedGeneration uint64 `json:"expected_generation"`
}

type BackendInitializationRequest struct {
	NodeID    string `json:"node_id"`
	BackendID string `json:"backend_id"`
	Profile   string `json:"profile,omitempty"`
}

type BackendInitializationJob = vllm.InitializationJob

// BackendLaunchOptionsRequest carries an operator's vLLM launch-option selection. A
// request with Options omitted is a read; the handlers distinguish the two by method.
type BackendLaunchOptionsRequest struct {
	NodeID    string             `json:"node_id"`
	BackendID string             `json:"backend_id"`
	Options   vllm.LaunchOptions `json:"options"`
}

type BackendLaunchOptions = vllm.LaunchOptions

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

// RoutingLinkState describes one direction between the anchor and a candidate.
// LoadIfUnloaded and RestoreAfterBorrow belong to whichever side is the helper.
type RoutingLinkState struct {
	Selected           bool `json:"selected"`
	LoadIfUnloaded     bool `json:"load_if_unloaded"`
	RestoreAfterBorrow bool `json:"restore_after_borrow"`
}

// RoutingCandidate is a model on another node that the anchor could lend work to or
// borrow work from. WeightsMatch reports whether it is the same checkpoint under a
// different config. A candidate where it is false answers requests with genuinely
// different output, and the router has no way to detect that, so the UI has to say
// so before it is chosen.
type RoutingCandidate struct {
	NodeID           string           `json:"node_id"`
	ModelID          string           `json:"model_id"`
	Filename         string           `json:"filename"`
	ModelHash        string           `json:"model_hash,omitempty"`
	ConfigHash       string           `json:"config_hash,omitempty"`
	ContextSize      int              `json:"context_size,omitempty"`
	Multimodal       bool             `json:"multimodal,omitempty"`
	WeightsMatch     bool             `json:"weights_match"`
	Eligible         bool             `json:"eligible"`
	IneligibleReason string           `json:"ineligible_reason,omitempty"`
	LendsTo          RoutingLinkState `json:"lends_to"`
	BorrowsFrom      RoutingLinkState `json:"borrows_from"`
}

type RoutingLinksResponse struct {
	Links      []routinggroups.Link    `json:"links"`
	Anchor     *routinggroups.Endpoint `json:"anchor,omitempty"`
	Candidates []RoutingCandidate      `json:"candidates,omitempty"`
}

type RoutingLinkChoice struct {
	NodeID             string `json:"node_id"`
	ModelID            string `json:"model_id"`
	LoadIfUnloaded     bool   `json:"load_if_unloaded"`
	RestoreAfterBorrow bool   `json:"restore_after_borrow"`
}

type RoutingLinksRequest struct {
	Anchor      routinggroups.Endpoint `json:"anchor"`
	LendsTo     []RoutingLinkChoice    `json:"lends_to"`
	BorrowsFrom []RoutingLinkChoice    `json:"borrows_from"`
}

type SeparateRuntimeSettings struct {
	RunSeparate bool     `json:"run_separate"`
	Triggers    []string `json:"triggers"`
}

type SeparateRuntimeRequest struct {
	NodeID   string                  `json:"node_id"`
	LocalID  string                  `json:"local_id"`
	Settings SeparateRuntimeSettings `json:"settings"`
}

// SeparateRuntimeResponse carries the effective settings for a config on a node
// plus the vocabulary the dialog offers. HasOverride is false when Settings is the
// inherited default rather than a saved per-node row.
type SeparateRuntimeResponse struct {
	NodeID      string                    `json:"node_id"`
	LocalID     string                    `json:"local_id"`
	Settings    SeparateRuntimeSettings   `json:"settings"`
	HasOverride bool                      `json:"has_override"`
	Inherited   SeparateRuntimeSettings   `json:"inherited"`
	Candidates  SeparateRuntimeCandidates `json:"candidates"`
}

type SeparateRuntimeCandidates struct {
	Lanes    []string `json:"lanes"`
	Families []string `json:"families"`
	Configs  []string `json:"configs"`
}

type ModelStateRequest struct {
	NodeID  string `json:"node_id"`
	LocalID string `json:"local_id"`
	Enabled bool   `json:"enabled"`
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
