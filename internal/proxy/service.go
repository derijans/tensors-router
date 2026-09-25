package proxy

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/backendmode"
	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
	"tensors-router/internal/downloader"
	"tensors-router/internal/ffmpeg"
	"tensors-router/internal/hardware"
	"tensors-router/internal/loadcapture"
	"tensors-router/internal/loaderrors"
	"tensors-router/internal/mcp"
	"tensors-router/internal/modelassets"
	"tensors-router/internal/modelstate"
	"tensors-router/internal/offloaddecisions"
	"tensors-router/internal/proxy/downloads"
	"tensors-router/internal/recipes"
	"tensors-router/internal/routinggroups"
	"tensors-router/internal/transportbody"
	"tensors-router/internal/vllm"
)

type Backend interface {
	URL() *url.URL
	ReloadConfig(ctx context.Context, filename string) error
	Restart(ctx context.Context) error
	Unload(ctx context.Context) error
	Healthy(ctx context.Context) bool
}

type backendExitReporter interface {
	BackendExitError() error
}

type backendHTTPClientProvider interface {
	HTTPClient() *http.Client
}

type BackendFamilyConfig struct {
	TextBackend          Backend
	EmbeddingsBackend    Backend
	ImageBackend         Backend
	TranscriptionBackend Backend
	SeparateBackend      separateBackendFactory
	Start                func(context.Context) error
	Stop                 func(context.Context) error
	StopPrimary          func(context.Context) error
}

type ModelCatalog interface {
	List() ([]catalog.Model, error)
	ListLLM() ([]catalog.Model, error)
	ListImages(activeConfigFilename string) ([]catalog.Model, error)
	Resolve(id string) (catalog.Model, bool, error)
	ResolveImage(id string, activeConfigFilename string) (catalog.Model, bool, error)
	ResolveActiveImage(activeConfigFilename string) (catalog.Model, bool, error)
}

type modelHashEnsurer interface {
	EnsureModelHashForFilename(filename string) (catalog.Model, bool, error)
}

type ServiceConfig struct {
	Backend                   Backend
	TextBackend               Backend
	ImageBackend              Backend
	BackendMode               string
	BackendFamilies           map[string]BackendFamilyConfig
	Catalog                   ModelCatalog
	Registry                  *cluster.Registry
	ClusterToken              string
	ClusterClient             *cluster.Client
	ClusterRole               string
	NodeID                    string
	NodeURL                   string
	MasterURL                 string
	SlaveURLs                 []string
	ConfigDir                 string
	MCPReconciler             *mcp.Reconciler
	MCPGateway                *mcp.Gateway
	FileRoots                 []string
	AssetIndex                *modelassets.Index
	RecipeStore               *recipes.Store
	BenchmarkStore            *routerbenchmark.Store
	ModelStateStore           *modelstate.Store
	AnalyticsStore            *routeranalytics.Store
	RoutingGroups             *routinggroups.Store
	SchedulingSampleWindow    time.Duration
	SchedulingMinSamples      int
	SchedulingBackendDepth    int
	SchedulingRefreshInterval time.Duration
	SchedulingGrantTTL        time.Duration
	SchedulingContextReserve  int
	OffloadRestoreDelay       time.Duration
	OffloadProbeIdle          time.Duration
	OffloadDecisionStore      *offloaddecisions.Store
	LoadCaptureStore          *loadcapture.Store
	LoadCaptureMaxOutputBytes int64
	LoadErrorStore            *loaderrors.Store
	VRAMAnalyticsEnabled      bool
	VRAMSource                hardware.VRAMSource
	VRAMSampleInterval        time.Duration
	Hardware                  hardware.Source
	Downloader                downloader.Service
	DownloaderCapability      downloader.Capability
	Logger                    *log.Logger
	Shutdown                  func()
	TransportLimits           transportbody.Limits
	MaxControlBodyBytes       int64
	ConcurrentAssetTransfers  int
	BackendBinaryPaths        map[string]string
	VLLM                      vllm.Service
	VLLMUnavailableReason     string
	VLLMDynamicLoRAEnabled    bool
	VLLMEEPEnabled            bool
	FFmpeg                    ffmpeg.Tool
	FFmpegScratchDir          string
	SeparateRuntimeLimit      int
}

type Service struct {
	backendMode               string
	backendFamilies           map[string]*backendFamily
	backendSwitch             *backendFamilySwitchState
	routes                    *routeTable
	separatePool              *separateRuntimePool
	webUI                     *webUIProxy
	catalog                   ModelCatalog
	registry                  *cluster.Registry
	clusterToken              string
	clusterClient             *cluster.Client
	clusterRole               string
	nodeID                    string
	nodeURL                   string
	masterURL                 string
	slaveURLs                 []string
	configDir                 string
	mcpReconciler             *mcp.Reconciler
	mcpGateway                *mcp.Gateway
	assets                    *assetManager
	recipeStore               *recipes.Store
	benchmarks                *benchmarkRunner
	modelStateStore           *modelstate.Store
	modelStateMu              sync.Mutex
	pendingModelUnloads       map[string]context.CancelFunc
	analytics                 *requestAnalytics
	routingGroups             *routinggroups.Store
	scheduler                 *scheduler
	routingLinks              atomic.Pointer[routingLinkIndex]
	loadCaptureStore          *loadcapture.Store
	loadCaptureMaxOutputBytes int64
	loadErrorStore            *loaderrors.Store
	hardware                  hardware.Source
	downloads                 *downloads.Handlers
	client                    *http.Client
	logger                    *log.Logger
	shutdown                  func()
	sdcppJobs                 *sdcppJobStore
	vllmResponses             *vllmResponseStore
	transportLimits           transportbody.Limits
	transportBudget           *transportbody.Budget
	maxControlBodyBytes       int64
	draining                  atomic.Bool
	sttTieRotation            roundRobin
	embeddingRotation         roundRobin
	backendBinaryPaths        map[string]string
	vllm                      vllm.Service
	vllmUnavailableReason     string
	vllmDynamicLoRAEnabled    bool
	vllmEEPEnabled            bool
	ffmpeg                    ffmpeg.Tool
	comfyVideoJobs            *comfyVideoJobStore
	nextRuntimeLease          atomic.Uint64

	backendRetryAttempts          int
	backendInferenceRetryAttempts int
	backendReadinessWait          time.Duration
	backendRetryDelay             time.Duration
	backendRetryMaxDelay          time.Duration
}

const (
	defaultBackendRetryAttempts = 300
	// Re-running inference is far more expensive than probing a readiness endpoint, so
	// the retry budget for a request that already produced a response is small. The large
	// budget above is for waiting on a model load, not for regenerating.
	defaultBackendInferenceRetryAttempts = 3
	// defaultBackendReadinessWait is how long a backend may sit with no model and no load
	// under way before the router concludes the reload was lost and re-issues it. It is
	// not a bound on load duration — a load that has announced itself is left alone for as
	// long as it needs — so this can be short.
	defaultBackendReadinessWait = 20 * time.Second
	// modelLoadAttempts is how many times a config load is issued before giving up. A lost
	// reload is recovered by repeating it, so this is the difference between a switch that
	// self-heals and one the client sees as a failure.
	modelLoadAttempts = 3
	// backendOutputGraceWait is how long to let the backend announce itself before probing
	// anyway. It is deliberately short: if the announcement is missed — the process was
	// already past it, or this backend words it differently — the cost must be a brief
	// pause, not a stalled load. Failure markers are still honoured throughout the probe
	// loop, so failing fast does not depend on this window.
	backendOutputGraceWait = 5 * time.Second
	// backendSelfRestartWait bounds how long to wait for a backend that restarts itself to
	// apply a config reload before concluding it needs a restart from us.
	backendSelfRestartWait       = 60 * time.Second
	defaultBackendRetryDelay     = 1 * time.Second
	defaultBackendRetryMaxDelay  = 2 * time.Second
	backendErrorBodyLimit        = 1024
	backendResponseMetadataLimit = 1 << 20
	modelOperationTimeout        = 15 * time.Minute
	assetLookupTimeout           = 30 * time.Second
	BackendModeKobold            = backendmode.Kobold
	BackendModeLlamaSDCPP        = backendmode.LlamaSDCPP
	BackendModeVLLM              = backendmode.VLLM
)

// llamaTextToSpeechUnsupportedMessage is the single wording for the split
// backend losing speech: llama.cpp removed --model-vocoder and --model-talker,
// and current llama-server exposes no speech endpoint at all, so this is a
// capability removal rather than a flag rename.
const llamaTextToSpeechUnsupportedMessage = "text-to-speech is not supported by the split backend: llama.cpp removed --model-vocoder and --model-talker, and llama-server has no /v1/audio/speech endpoint; use backend_mode kobold or vllm for text-to-speech"

type replayReadCloser struct {
	io.Reader
	closer io.Closer
}

func (body replayReadCloser) Close() error {
	return body.closer.Close()
}

type releaseReadCloser struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (body *releaseReadCloser) Close() error {
	err := body.ReadCloser.Close()
	body.once.Do(body.release)
	return err
}
