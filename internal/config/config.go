package config

import (
	"time"

	"tensors-router/internal/offloadsettings"
)

type Config struct {
	Security    SecurityConfig
	Server      ServerConfig
	Auth        AuthConfig
	Models      ModelsConfig
	Backend     BackendConfig
	Kobold      KoboldConfig
	Llama       NativeServerConfig
	SDCPP       NativeServerConfig
	WhisperCPP  NativeServerConfig
	VLLM        VLLMConfig
	Logging     LoggingConfig
	Updates     UpdatesConfig
	Downloader  DownloaderConfig
	Cluster     ClusterConfig
	Analytics   AnalyticsConfig
	Diagnostics DiagnosticsConfig
	Limits      LimitsConfig
	MCP         MCPConfig
	FFmpeg      FFmpegConfig
	Warnings    []string
}

type ServerConfig struct {
	Bind         string
	AllowedCIDRs []string
}

type AuthConfig struct {
	InferenceKeys []string
	AdminKeys     []string
	BearerKeys    []string
}

type ModelsConfig struct {
	ConfigDir                string
	StartupModel             string
	FileRoots                []string
	SharedDir                string
	HashWorkers              int
	ConcurrentAssetTransfers int
}

type BackendConfig struct {
	Mode string
}

type MCPConfig struct {
	Enabled   bool
	Directory string
}

// FFmpegConfig locates an optional ffmpeg binary used to remux backend video
// output into MP4 and to convert non-WAV audio for transcription. Missing
// ffmpeg is never a startup failure; requests that need it fail explicitly.
//
// ScratchDir holds finished videos until a client collects them. It is
// operator-controlled because container deployments run read-only with a
// small /tmp: pointing it at the data volume is what makes the router's size
// caps the effective limit rather than the tmpfs size.
type FFmpegConfig struct {
	BinaryPath string
	ScratchDir string
}

type KoboldConfig struct {
	BackendURL              string
	EmbeddingsBackendURL    string
	embeddingsBackendURLSet bool
	BinaryPath              string
	DataDir                 string
	Multiuser               int
	ExtraArgs               []string
	Quiet                   bool
	SkipLauncher            bool
	NoModel                 bool
	HideWindow              bool
}

type NativeServerConfig struct {
	BackendURL              string
	EmbeddingsBackendURL    string
	embeddingsBackendURLSet bool
	BinaryPath              string
	DataDir                 string
	ExtraArgs               []string
	HideWindow              bool
}

type LoggingConfig struct {
	Mode              string
	Enabled           bool
	BackendLogsToDisk bool
	legacyEnabledSet  bool
	modeSet           bool
}

type UpdatesConfig struct {
	Enabled                 bool
	CheckInterval           time.Duration
	IncludePrereleases      bool
	TUFRepositoryURL        string
	TUFRootPath             string
	BinaryURL               string
	BinarySHA256            string
	BinaryRepositoryURL     string
	BinaryAssetGlob         string
	LlamaBinaryURL          string
	LlamaSHA256             string
	LlamaRepositoryURL      string
	LlamaAssetGlob          string
	SDCPPBinaryURL          string
	SDCPPSHA256             string
	SDCPPRepositoryURL      string
	SDCPPAssetGlob          string
	WhisperCPPBinaryURL     string
	WhisperCPPSHA256        string
	WhisperCPPRepositoryURL string
	WhisperCPPAssetGlob     string
}

type DownloaderConfig struct {
	Enabled        bool
	BinaryLocation string
	ConfigPath     string
}

type VLLMConfig struct {
	BinaryLocation     string
	DataDir            string
	Profile            string
	ManifestPath       string
	ManifestSHA256     string
	ManifestSize       int64
	TUFRepositoryURL   string
	TUFRootPath        string
	DynamicLoRAEnabled bool
	EEPEnabled         bool
	TrustRemoteCode    bool
	ExternalTools      bool
	// AllowUnverifiedInstall opts into installing vLLM straight from PyPI with no
	// manifest and no digest pinning, when no TUF-published or operator-pinned
	// manifest is available. Off by default: this is a real reduction in
	// supply-chain integrity, not a convenience toggle.
	AllowUnverifiedInstall  bool
	UnverifiedVLLMVersion   string
	UnverifiedPythonVersion string
	UnverifiedIndexURL      string
	UnverifiedExtraIndexURL string
	// OCIRunAsImageUser keeps an OCI vLLM runtime on the image own user instead of
	// the host user, for vendor images that are unusable as a non-root user.
	OCIRunAsImageUser bool
}

type BackendUpdateSource struct {
	BinaryURL     string
	SHA256        string
	RepositoryURL string
	AssetGlob     string
}

func (updates UpdatesConfig) KoboldSource() BackendUpdateSource {
	return BackendUpdateSource{BinaryURL: updates.BinaryURL, SHA256: updates.BinarySHA256, RepositoryURL: updates.BinaryRepositoryURL, AssetGlob: updates.BinaryAssetGlob}
}

func (updates UpdatesConfig) LlamaSource() BackendUpdateSource {
	return BackendUpdateSource{BinaryURL: updates.LlamaBinaryURL, SHA256: updates.LlamaSHA256, RepositoryURL: updates.LlamaRepositoryURL, AssetGlob: updates.LlamaAssetGlob}
}

func (updates UpdatesConfig) SDCPPSource() BackendUpdateSource {
	return BackendUpdateSource{BinaryURL: updates.SDCPPBinaryURL, SHA256: updates.SDCPPSHA256, RepositoryURL: updates.SDCPPRepositoryURL, AssetGlob: updates.SDCPPAssetGlob}
}

func (updates UpdatesConfig) WhisperCPPSource() BackendUpdateSource {
	return BackendUpdateSource{BinaryURL: updates.WhisperCPPBinaryURL, SHA256: updates.WhisperCPPSHA256, RepositoryURL: updates.WhisperCPPRepositoryURL, AssetGlob: updates.WhisperCPPAssetGlob}
}

type ClusterConfig struct {
	Role            string
	NodeID          string
	PublicURL       string
	MasterURL       string
	SlaveURLs       []string
	Token           string
	StoreDir        string
	DatabasePath    string
	SyncInterval    time.Duration
	HealthInterval  time.Duration
	ControlTimeout  time.Duration
	SyncConcurrency int

	LendingFileValues offloadsettings.Values
}

type AnalyticsConfig struct {
	Enabled                 bool
	VRAMEnabled             bool
	LoadCaptureEnabled      bool
	LoadCaptureDatabasePath string
	LoadCaptureMaxOutputMB  int64
	FlushInterval           time.Duration
	DatabasePath            string
	RawRetention            time.Duration
	VRAMSampleInterval      time.Duration
}

// DiagnosticsConfig owns the pre-load error store, a standalone SQLite file
// kept on by default so failures logged before a backend loads stay queryable
// even where analytics is disabled.
type DiagnosticsConfig struct {
	Enabled      bool
	DatabasePath string
	Retention    time.Duration
	MaxOutputKB  int64
}
