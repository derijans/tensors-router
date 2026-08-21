package vllm

import "time"

const (
	BackendID = "vllm"

	LifecycleCompanionMissing = "companion_missing"
	LifecycleUnsupported      = "unsupported"
	LifecycleNeedsInit        = "needs_init"
	LifecycleInitializing     = "initializing"
	LifecycleReady            = "ready"
	LifecycleFailed           = "failed"

	JobQueued    = "queued"
	JobRunning   = "running"
	JobCompleted = "completed"
	JobFailed    = "failed"
	JobCancelled = "cancelled"
)

type State struct {
	LifecycleState           string `json:"lifecycle_state"`
	SelectedProfile          string `json:"selected_profile,omitempty"`
	DetectedProfile          string `json:"detected_profile,omitempty"`
	RuntimeVersion           string `json:"runtime_version,omitempty"`
	InitializationJobID      string `json:"initialization_job_id,omitempty"`
	InitializationPhase      string `json:"initialization_phase,omitempty"`
	InitializationBytes      int64  `json:"initialization_bytes,omitempty"`
	InitializationTotalBytes int64  `json:"initialization_total_bytes,omitempty"`
	ManifestTrust            string        `json:"manifest_trust,omitempty"`
	LaunchOptions            LaunchOptions `json:"launch_options"`
	Error                    string        `json:"error,omitempty"`
	Retryable                bool          `json:"retryable,omitempty"`
}

// LaunchOptions are operator-selected environment switches applied every time a vLLM
// runtime process starts. They persist in the companion's data directory, so a choice
// survives a router or companion restart, and they take effect on the next runtime
// launch - which is why setting them unloads whatever is currently running.
type LaunchOptions struct {
	// HubOffline controls HF_HUB_OFFLINE. Offline is the safe default: it keeps a
	// running model from reaching Hugging Face, so only the local pinned snapshot is
	// ever used. Turning it off lets vLLM resolve missing files over the network.
	HubOffline bool `json:"hf_hub_offline"`
	// TransformersOffline controls TRANSFORMERS_OFFLINE.
	TransformersOffline bool `json:"transformers_offline"`
	// DatasetsOffline controls HF_DATASETS_OFFLINE.
	DatasetsOffline bool `json:"hf_datasets_offline"`
}

// DefaultLaunchOptions keeps the fully offline behaviour that was previously hardcoded
// into the runtime environment, so an existing deployment behaves identically until an
// operator deliberately changes something.
func DefaultLaunchOptions() LaunchOptions {
	return LaunchOptions{HubOffline: true, TransformersOffline: true, DatasetsOffline: true}
}

type InitializationJob struct {
	JobID           string    `json:"job_id"`
	BackendID       string    `json:"backend_id"`
	State           string    `json:"state"`
	SelectedProfile string    `json:"selected_profile,omitempty"`
	DetectedProfile string    `json:"detected_profile,omitempty"`
	ManifestSHA256  string    `json:"manifest_sha256,omitempty"`
	ManifestTrust   string    `json:"manifest_trust,omitempty"`
	Phase           string    `json:"phase,omitempty"`
	CompletedBytes  int64     `json:"completed_bytes"`
	TotalBytes      int64     `json:"total_bytes"`
	Error           string    `json:"error,omitempty"`
	Retryable       bool      `json:"retryable,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type InitRequest struct {
	Profile string `json:"profile,omitempty"`
}

type RuntimeKind string

const (
	RuntimeGeneration RuntimeKind = "generation"
	RuntimePooling    RuntimeKind = "pooling"
	RuntimeSpeech     RuntimeKind = "speech"
)

type RuntimeStatus struct {
	Kind       RuntimeKind `json:"kind"`
	Running    bool        `json:"running"`
	Healthy    bool        `json:"healthy"`
	SocketPath string      `json:"socket_path,omitempty"`
	ModelID    string      `json:"model_id,omitempty"`
	Version    string      `json:"version,omitempty"`
	Error      string      `json:"error,omitempty"`
	Logs       string      `json:"logs,omitempty"`
}

type RuntimeLoadRequest struct {
	Kind       RuntimeKind `json:"kind"`
	ConfigPath string      `json:"config_path"`
}

type Detection struct {
	OS            string          `json:"os"`
	Architecture  string          `json:"architecture"`
	Devices       []string        `json:"devices"`
	Prerequisites map[string]bool `json:"prerequisites"`
}

type Manifest struct {
	SchemaVersion int       `json:"schema_version"`
	Release       string    `json:"release"`
	Profiles      []Profile `json:"profiles"`
}

type Profile struct {
	ID               string            `json:"id"`
	Priority         int               `json:"priority"`
	VLLMVersion      string            `json:"vllm_version"`
	PythonVersion    string            `json:"python_version"`
	PluginVersions   map[string]string `json:"plugin_versions,omitempty"`
	InstallMethod    string            `json:"install_method"`
	OCIImage         string            `json:"oci_image,omitempty"`
	OperatingSystems []string          `json:"operating_systems"`
	Architectures    []string          `json:"architectures"`
	Devices          []string          `json:"devices"`
	Prerequisites    []Prerequisite    `json:"prerequisites,omitempty"`
	Artifacts        []Artifact        `json:"artifacts"`
}

type Prerequisite struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type Artifact struct {
	Name           string `json:"name"`
	URL            string `json:"url"`
	Size           int64  `json:"size"`
	SHA256         string `json:"sha256"`
	Role           string `json:"role"`
	ArchiveFormat  string `json:"archive_format,omitempty"`
	UnpackedSize   int64  `json:"unpacked_size,omitempty"`
	ExecutablePath string `json:"executable_path,omitempty"`
}

type ArtifactAuthorization struct {
	Length int64  `json:"length"`
	SHA256 string `json:"sha256"`
}

type Progress struct {
	Phase          string
	CompletedBytes int64
	TotalBytes     int64
}

type VLLMModelConfig struct {
	Snapshot            SnapshotIdentity `json:"snapshot"`
	Runner              string           `json:"runner,omitempty"`
	Task                string           `json:"task,omitempty"`
	ServedNames         []string         `json:"served_names,omitempty"`
	StaticAdapters      []StaticAdapter  `json:"static_adapters,omitempty"`
	Settings            CommonSettings   `json:"settings,omitempty"`
	ServeArgs           []string         `json:"serve_args,omitempty"`
	TrustRemoteCode     bool             `json:"trust_remote_code,omitempty"`
	ExternalToolServers []string         `json:"external_tool_servers,omitempty"`
}

type SnapshotIdentity struct {
	Path       string `json:"path"`
	Repository string `json:"repository,omitempty"`
	Revision   string `json:"revision,omitempty"`
	TreeDigest string `json:"tree_digest"`
}

type StaticAdapter struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	TreeDigest string `json:"tree_digest"`
}

type CommonSettings struct {
	DType                string  `json:"dtype,omitempty"`
	MaxModelLength       int     `json:"max_model_length,omitempty"`
	GPUUtilization       float64 `json:"gpu_memory_utilization,omitempty"`
	TensorParallelSize   int     `json:"tensor_parallel_size,omitempty"`
	PipelineParallelSize int     `json:"pipeline_parallel_size,omitempty"`
	DataParallelSize     int     `json:"data_parallel_size,omitempty"`
	MaxNumberSequences   int     `json:"max_number_sequences,omitempty"`
	EnableChunkedPrefill *bool   `json:"enable_chunked_prefill,omitempty"`
}
