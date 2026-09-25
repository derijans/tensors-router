package proxy

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"tensors-router/internal/backendmode"
	"tensors-router/internal/cluster"
	"tensors-router/internal/hardware"
	"tensors-router/internal/proxy/downloads"
	"tensors-router/internal/transportbody"
)

func NewService(config ServiceConfig) *Service {
	logger := config.Logger
	if logger == nil {
		logger = log.Default()
	}
	backendMode, err := backendmode.Resolve("", config.BackendMode)
	if err != nil {
		backendMode = BackendModeKobold
	}
	clusterRole := strings.TrimSpace(config.ClusterRole)
	if clusterRole == "" {
		clusterRole = cluster.RoleStandalone
	}
	vramSampleInterval := config.VRAMSampleInterval
	if vramSampleInterval <= 0 {
		vramSampleInterval = 250 * time.Millisecond
	}
	vramSource := config.VRAMSource
	var vramSampler *hardware.VRAMSampler
	if config.VRAMAnalyticsEnabled && vramSource == nil {
		vramSampler = hardware.NewVRAMSampler(hardware.NewVRAMReader(), vramSampleInterval)
		vramSource = vramSampler
	}
	maxControlBodyBytes := config.MaxControlBodyBytes
	if config.LoadCaptureMaxOutputBytes <= 0 {
		config.LoadCaptureMaxOutputBytes = 64 * 1024 * 1024
	}
	if maxControlBodyBytes <= 0 {
		maxControlBodyBytes = 8 * transportbody.MiB
	}
	concurrentAssetTransfers := config.ConcurrentAssetTransfers
	if concurrentAssetTransfers <= 0 {
		concurrentAssetTransfers = 2
	}
	nodeID := strings.TrimSpace(config.NodeID)
	if nodeID == "" {
		nodeID = "local"
	}
	backendFamilies := backendFamiliesFromConfig(config, backendMode)
	if backendFamilies[backendMode] == nil {
		for mode := range backendFamilies {
			backendMode = mode
			break
		}
	}
	service := &Service{
		backendMode:     backendMode,
		backendFamilies: backendFamilies,
		backendSwitch: &backendFamilySwitchState{
			changed: make(chan struct{}),
			mode:    backendMode,
		},
		catalog:                   config.Catalog,
		registry:                  config.Registry,
		clusterToken:              config.ClusterToken,
		clusterRole:               clusterRole,
		nodeID:                    nodeID,
		nodeURL:                   strings.TrimSpace(config.NodeURL),
		masterURL:                 strings.TrimSpace(config.MasterURL),
		slaveURLs:                 append([]string{}, config.SlaveURLs...),
		configDir:                 strings.TrimSpace(config.ConfigDir),
		mcpReconciler:             config.MCPReconciler,
		mcpGateway:                config.MCPGateway,
		recipeStore:               config.RecipeStore,
		modelStateStore:           config.ModelStateStore,
		pendingModelUnloads:       map[string]context.CancelFunc{},
		routingGroups:             config.RoutingGroups,
		loadCaptureStore:          config.LoadCaptureStore,
		loadCaptureMaxOutputBytes: config.LoadCaptureMaxOutputBytes,
		loadErrorStore:            config.LoadErrorStore,
		hardware:                  config.Hardware,
		logger:                    logger,
		shutdown:                  config.Shutdown,
		sdcppJobs:                 newSdcppJobStore(),
		vllmResponses:             newVLLMResponseStore(),
		transportLimits:           config.TransportLimits.Normalized(),
		maxControlBodyBytes:       maxControlBodyBytes,
		backendBinaryPaths:        copyStringMap(config.BackendBinaryPaths),
		vllm:                      config.VLLM,
		vllmUnavailableReason:     strings.TrimSpace(config.VLLMUnavailableReason),
		vllmDynamicLoRAEnabled:    config.VLLMDynamicLoRAEnabled,
		vllmEEPEnabled:            config.VLLMEEPEnabled,
		ffmpeg:                    config.FFmpeg,
		comfyVideoJobs:            newComfyVideoJobStore(config.FFmpegScratchDir),
		client: &http.Client{
			Timeout:       0,
			CheckRedirect: returnRedirectToCaller,
		},
		backendRetryAttempts:          defaultBackendRetryAttempts,
		backendInferenceRetryAttempts: defaultBackendInferenceRetryAttempts,
		backendReadinessWait:          defaultBackendReadinessWait,
		backendRetryDelay:             defaultBackendRetryDelay,
		backendRetryMaxDelay:          defaultBackendRetryMaxDelay,
	}
	service.transportBudget = transportbody.NewBudget(service.transportLimits.MemoryBudgetBytes)
	if service.clusterClient == nil {
		service.clusterClient = cluster.NewClient(config.ClusterToken)
	}
	if service.hardware == nil {
		service.hardware = hardware.NewCache()
	}
	service.analytics = &requestAnalytics{
		store:        config.AnalyticsStore,
		vramEnabled:  config.VRAMAnalyticsEnabled,
		vramSource:   vramSource,
		vramSampler:  vramSampler,
		vramInterval: vramSampleInterval,
		nodeID:       nodeID,
		loadSection:  service.loadAnalyticsSection,
	}
	service.webUI = newWebUIProxy(service, service.analytics)
	service.benchmarks = newBenchmarkRunner(service, config.BenchmarkStore, logger)
	service.downloads = downloads.New(downloadDeps{service: service}, config.Downloader, config.DownloaderCapability)
	service.assets = newAssetManager(service, assetManagerConfig{
		index:              config.AssetIndex,
		fileRoots:          config.FileRoots,
		downloader:         config.Downloader,
		concurrentTransfer: concurrentAssetTransfers,
		logger:             logger,
	})
	if config.ClusterClient != nil {
		service.clusterClient = config.ClusterClient
	}
	if err := service.clusterClient.AllowBaseURLs(service.knownClusterTargets()...); err != nil {
		service.logger.Printf("cluster target setup failed: %v", err)
	}
	service.separatePool = newSeparateRuntimePool(config.SeparateRuntimeLimit)
	service.scheduler = newScheduler(service, service.analytics, decisionRecorderOrNil(config.OffloadDecisionStore), schedulingSettings{
		sampleWindow:    config.SchedulingSampleWindow,
		minSamples:      config.SchedulingMinSamples,
		backendDepth:    config.SchedulingBackendDepth,
		refreshInterval: config.SchedulingRefreshInterval,
		grantTTL:        config.SchedulingGrantTTL,
		contextReserve:  config.SchedulingContextReserve,
		restoreDelay:    config.OffloadRestoreDelay,
		probeIdle:       config.OffloadProbeIdle,
	}, logger)
	service.installStoredRoutingLinks(context.Background())
	service.routes = newRouteTable(service.requireClusterToken, service.controlRoutes(), service.siteRoutes(), service.nodeRoutes(), service.downloads.Routes(), service.benchmarks.routes(), service.assets.routes(), service.webUI.routes())
	return service
}

func backendFamiliesFromConfig(config ServiceConfig, defaultMode string) map[string]*backendFamily {
	configs := map[string]BackendFamilyConfig{}
	for mode, family := range config.BackendFamilies {
		normalizedMode := backendmode.Normalize(mode)
		if !backendmode.Valid(normalizedMode) {
			continue
		}
		configs[normalizedMode] = family
	}
	if len(configs) == 0 {
		textBackend := config.TextBackend
		if textBackend == nil {
			textBackend = config.Backend
		}
		configs[defaultMode] = BackendFamilyConfig{
			TextBackend:          textBackend,
			EmbeddingsBackend:    textBackend,
			ImageBackend:         config.ImageBackend,
			TranscriptionBackend: textBackend,
		}
	}

	families := map[string]*backendFamily{}
	for mode, familyConfig := range configs {
		family := newBackendFamily(mode, familyConfig)
		if family != nil {
			families[mode] = family
		}
	}
	return families
}

func newBackendFamily(mode string, config BackendFamilyConfig) *backendFamily {
	textBackend := config.TextBackend
	if textBackend == nil {
		return nil
	}
	imageBackend := config.ImageBackend
	if imageBackend == nil || mode != BackendModeLlamaSDCPP {
		imageBackend = textBackend
	}
	sharedState := newActiveConfigState()
	textRuntime := &backendRuntime{backend: textBackend, state: sharedState, mode: mode, name: mode + "-text"}
	embeddingsRuntime := textRuntime
	if config.EmbeddingsBackend != nil && config.EmbeddingsBackend != textBackend {
		embeddingsRuntime = &backendRuntime{backend: config.EmbeddingsBackend, state: newActiveConfigState(), mode: mode, name: mode + "-embeddings"}
	}
	imageRuntime := textRuntime
	transcriptionRuntime := textRuntime
	if mode == BackendModeLlamaSDCPP && config.ImageBackend != nil {
		imageRuntime = &backendRuntime{backend: imageBackend, state: newActiveConfigState(), mode: mode, name: mode + "-image"}
	}
	if config.TranscriptionBackend != nil && config.TranscriptionBackend != textBackend {
		name := mode + "-speech"
		if mode == BackendModeLlamaSDCPP {
			name = mode + "-transcription"
		}
		transcriptionRuntime = &backendRuntime{backend: config.TranscriptionBackend, state: newActiveConfigState(), mode: mode, name: name}
	}
	return &backendFamily{
		mode:                 mode,
		textRuntime:          textRuntime,
		embeddingsRuntime:    embeddingsRuntime,
		imageRuntime:         imageRuntime,
		transcriptionRuntime: transcriptionRuntime,
		separateBackend:      config.SeparateBackend,
		start:                config.Start,
		stop:                 config.Stop,
		stopPrimary:          config.StopPrimary,
	}
}

func (service *Service) knownClusterTargets() []string {
	values := []string{service.nodeURL, service.masterURL}
	if service.registry != nil {
		values = append(values, service.registry.NodeURLs()...)
	}
	return values
}

func (service *Service) PreloadModel(ctx context.Context, modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}
	model, ok, err := service.catalog.Resolve(modelID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("startup model %q was not found", modelID)
	}
	enabled, err := service.localModelEnabled(ctx, model.ID)
	if err != nil {
		return err
	}
	if !enabled {
		return fmt.Errorf("startup model %q is disabled", modelID)
	}
	if !modelSupportsTextLane(model) {
		return fmt.Errorf("startup model %q is not a text-lane model", modelID)
	}
	modelBackendMode, err := service.catalogModelBackendMode(model)
	if err != nil {
		return err
	}

	modelContext, cancelModelContext := context.WithTimeout(ctx, modelOperationTimeout)
	defer cancelModelContext()

	_, release, _, err := service.acquireModelConfigForBackendMode(modelBackendMode, modelContext, model.ID, model.Filename, readinessText, false)
	if err != nil {
		return err
	}
	release()
	return nil
}

func (service *Service) BeginDrain() {
	service.draining.Store(true)
}

func (service *Service) Draining() bool {
	return service.draining.Load()
}

func (service *Service) Close(ctx context.Context) error {
	err := service.scheduler.close(ctx)
	err = errors.Join(err, service.assets.close(ctx))
	service.vllmResponses.close()
	service.modelStateMu.Lock()
	for _, cancel := range service.pendingModelUnloads {
		cancel()
	}
	service.pendingModelUnloads = map[string]context.CancelFunc{}
	service.modelStateMu.Unlock()
	service.closeSeparateRuntimes(ctx)
	err = errors.Join(err, service.analytics.close(ctx))
	if service.modelStateStore != nil {
		err = errors.Join(err, service.modelStateStore.Close())
	}
	return err
}
