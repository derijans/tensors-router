package downloader

import "time"

type Config struct {
	Storage     StorageConfig
	HuggingFace HuggingFaceConfig
	Downloads   DownloadsConfig
	Scanning    ScanningConfig
	Hardware    HardwareConfig
	Logging     LoggingConfig
}

type StorageConfig struct {
	Root               string
	StateDir           string
	DatabasePath       string
	FreeSpaceReserveGB int64
}

type HuggingFaceConfig struct {
	Token string
}

type DownloadsConfig struct {
	ConcurrentJobs  int
	ConcurrentFiles int
	RetryLimit      int
	Timeout         time.Duration
}

type ScanningConfig struct {
	HashWorkers       int
	WriteHashSidecars bool
}

type HardwareConfig struct {
	DefaultContext      int
	VRAMReserveMB       int64
	SafetyMarginPercent int
}

type LoggingConfig struct {
	Mode string
	Path string
}

type Capability struct {
	Enabled               bool   `json:"enabled"`
	Present               bool   `json:"present"`
	Working               bool   `json:"working"`
	Available             bool   `json:"available"`
	Configured            bool   `json:"configured"`
	ConfiguredToken       bool   `json:"configured_token"`
	StorageRoot           string `json:"storage_root,omitempty"`
	FreeBytes             int64  `json:"free_bytes,omitempty"`
	FreeSpaceReserveBytes int64  `json:"free_space_reserve_bytes,omitempty"`
	Reason                string `json:"reason,omitempty"`
	Error                 string `json:"error,omitempty"`
}

type DeviceCapability struct {
	Backend               string `json:"backend"`
	DeviceID              string `json:"device_id"`
	Name                  string `json:"name"`
	TotalVRAMBytes        int64  `json:"total_vram_bytes"`
	Architecture          string `json:"architecture,omitempty"`
	BackendVersion        string `json:"backend_version,omitempty"`
	SplitOffloadSupported bool   `json:"split_offload_supported"`
}

type HardwareFit struct {
	Status                 string `json:"status"`
	BackendSupported       string `json:"backend_supported"`
	EstimatedWeightBytes   int64  `json:"estimated_weight_bytes"`
	EstimatedKVBytes       int64  `json:"estimated_kv_bytes"`
	EstimatedOverheadBytes int64  `json:"estimated_overhead_bytes"`
	MaximumContext         int    `json:"maximum_context"`
	RequestedContext       int    `json:"requested_context"`
	LargestSingleGPUBytes  int64  `json:"largest_single_gpu_bytes"`
	SplitGPUBytes          int64  `json:"split_gpu_bytes"`
	Reason                 string `json:"reason,omitempty"`
}

type File struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	LFSHash  string `json:"lfs_sha256,omitempty"`
	GitOID   string `json:"git_oid,omitempty"`
	XetHash  string `json:"xet_hash,omitempty"`
	Unsafe   string `json:"unsafe_status,omitempty"`
	Required bool   `json:"required"`
	Reason   string `json:"reason,omitempty"`
}

type RepositoryDetails struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	Commit     string `json:"commit"`
	License    string `json:"license,omitempty"`
	Gated      string `json:"gated,omitempty"`
	Security   string `json:"security_status,omitempty"`
	Files      []File `json:"files"`
}

type PlannedFile struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Required bool   `json:"required"`
	Reason   string `json:"reason"`
	LFSHash  string `json:"lfs_sha256,omitempty"`
}

type DownloadPlan struct {
	Repository    string        `json:"repository"`
	Revision      string        `json:"revision"`
	Commit        string        `json:"commit"`
	Files         []PlannedFile `json:"files"`
	TotalBytes    int64         `json:"total_bytes"`
	Destination   string        `json:"destination"`
	UnsafeWarning bool          `json:"unsafe_warning"`
	Snapshot      bool          `json:"snapshot,omitempty"`
}

type JobState string

const (
	JobQueued    JobState = "queued"
	JobRunning   JobState = "running"
	JobPaused    JobState = "paused"
	JobCancelled JobState = "cancelled"
	JobFailed    JobState = "failed"
	JobCompleted JobState = "completed"
)

func (state JobState) Terminal() bool {
	return state == JobCancelled || state == JobFailed || state == JobCompleted
}

type DownloadJob struct {
	ID             string    `json:"id"`
	NodeID         string    `json:"node_id,omitempty"`
	Repository     string    `json:"repository"`
	Revision       string    `json:"revision"`
	Commit         string    `json:"commit"`
	State          JobState  `json:"state"`
	TotalBytes     int64     `json:"total_bytes"`
	CompletedBytes int64     `json:"completed_bytes"`
	Error          string    `json:"error,omitempty"`
	Snapshot       bool      `json:"snapshot,omitempty"`
	TreeSHA256     string    `json:"tree_sha256,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Files          []JobFile `json:"files,omitempty"`
}

type JobFile struct {
	Path           string `json:"path"`
	Reason         string `json:"reason"`
	ExpectedSHA256 string `json:"expected_sha256,omitempty"`
	Size           int64  `json:"size"`
	CompletedBytes int64  `json:"completed_bytes"`
	State          string `json:"state"`
	Error          string `json:"error,omitempty"`
}

type ArtifactRecord struct {
	Path               string    `json:"path"`
	SHA256             string    `json:"sha256"`
	Size               int64     `json:"size"`
	ModifiedUnixNano   int64     `json:"modified_unix_nano"`
	Repository         string    `json:"repository,omitempty"`
	RepositoryPath     string    `json:"repository_path,omitempty"`
	Revision           string    `json:"revision,omitempty"`
	VerificationSource string    `json:"verification_source"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type SearchRequest struct {
	Query              string   `json:"query"`
	Author             string   `json:"author,omitempty"`
	Filters            []string `json:"filters,omitempty"`
	PipelineTag        string   `json:"pipeline_tag,omitempty"`
	NumParameters      string   `json:"num_parameters,omitempty"`
	Apps               []string `json:"apps,omitempty"`
	Gated              string   `json:"gated,omitempty"`
	Inference          string   `json:"inference,omitempty"`
	InferenceProviders []string `json:"inference_providers,omitempty"`
	TrainedDatasets    []string `json:"trained_datasets,omitempty"`
	Sort               string   `json:"sort,omitempty"`
	Direction          string   `json:"direction,omitempty"`
	Limit              int      `json:"limit,omitempty"`
	Cursor             string   `json:"cursor,omitempty"`
	Tags               []string `json:"tags,omitempty"`
}

type SearchResult struct {
	ID        string    `json:"id"`
	Author    string    `json:"author,omitempty"`
	Downloads int64     `json:"downloads"`
	Likes     int64     `json:"likes"`
	Gated     string    `json:"gated,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

type SearchPage struct {
	Results    []SearchResult `json:"results"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type RepositoryRequest struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision,omitempty"`
	Token      string `json:"token,omitempty"`
}

type PlanRequest struct {
	Repository string   `json:"repository"`
	Revision   string   `json:"revision,omitempty"`
	Files      []string `json:"files,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	Token      string   `json:"token,omitempty"`
}

type CreateJobRequest struct {
	Repository     string   `json:"repository"`
	Revision       string   `json:"revision,omitempty"`
	Files          []string `json:"files"`
	Mode           string   `json:"mode,omitempty"`
	Token          string   `json:"token,omitempty"`
	ConfirmUnsafe  bool     `json:"confirm_unsafe"`
	ConfirmReplace bool     `json:"confirm_replace"`
}
