package main

import (
	"tensors-router/internal/auth"
	"tensors-router/internal/config"
	"tensors-router/internal/hardware"
	"tensors-router/internal/offloadsettings"
	"tensors-router/internal/proxy"
	"tensors-router/internal/transportbody"
)

func (runtime *serveRuntime) createBackendFamilies() error {
	families, shutdowns, err := createBackends(runtime.cfg, runtime.mcpReconciler, llamaVideoFFmpegDir(runtime.ffmpeg))
	if err != nil {
		return err
	}
	for mode, family := range vllmBackendFamilies(runtime.vllm, runtime.cfg.Models.ConfigDir) {
		families[mode] = family
	}
	runtime.backendFamilies = families
	runtime.shutdownBackends = shutdowns
	return nil
}

func newAuthPolicy(cfg config.Config) (*auth.Policy, error) {
	return auth.NewPolicy(auth.PolicyConfig{
		AllowedCIDRs:  cfg.Server.AllowedCIDRs,
		Profile:       cfg.Security.Profile,
		InferenceKeys: cfg.Auth.InferenceKeys,
		AdminKeys:     cfg.Auth.AdminKeys,
		ClusterToken:  cfg.Cluster.Token,
	})
}

func (runtime *serveRuntime) startRouter(shutdownRequested chan<- struct{}) {
	fileRoots := modelFileRoots(runtime.cfg.Models.FileRoots, runtime.downloaderCapability)
	runtime.startupLogger.Printf("model file roots=%q", fileRoots)
	runtime.router = proxy.NewService(runtime.serviceConfig(fileRoots, shutdownRequested))
	runtime.router.RecordConfigWarnings(append(append([]string{}, runtime.cfg.Warnings...), runtime.stores.handle.Warnings()...))
}

func (runtime *serveRuntime) serviceConfig(fileRoots []string, shutdownRequested chan<- struct{}) proxy.ServiceConfig {
	cfg := runtime.cfg
	stores := runtime.stores
	return proxy.ServiceConfig{
		BackendMode:              cfg.Backend.Mode,
		BackendFamilies:          runtime.backendFamilies,
		Catalog:                  runtime.catalog,
		Registry:                 runtime.registry,
		ClusterToken:             cfg.Cluster.Token,
		ClusterClient:            runtime.clusterClient,
		ClusterRole:              cfg.Cluster.Role,
		NodeID:                   cfg.Cluster.NodeID,
		NodeURL:                  cfg.Cluster.PublicURL,
		MasterURL:                cfg.Cluster.MasterURL,
		SlaveURLs:                cfg.Cluster.SlaveURLs,
		ConfigDir:                cfg.Models.ConfigDir,
		MCPReconciler:            runtime.mcpReconciler,
		MCPGateway:               runtime.mcpGateway,
		FileRoots:                fileRoots,
		AssetIndex:               runtime.assetIndex,
		ConcurrentAssetTransfers: cfg.Models.ConcurrentAssetTransfers,
		BackendBinaryPaths: map[string]string{
			"koboldcpp":      cfg.Kobold.BinaryPath,
			"llama-server":   cfg.Llama.BinaryPath,
			"sd-server":      cfg.SDCPP.BinaryPath,
			"whisper-server": cfg.WhisperCPP.BinaryPath,
			"vllm":           vllmBinaryPathForState(runtime.configPath, cfg.VLLM.BinaryLocation),
		},
		RecipeStore:               stores.recipes,
		BenchmarkStore:            stores.benchmarks,
		ModelStateStore:           stores.modelState,
		RoutingGroups:             stores.routingGroups,
		LendingFileValues:         cfg.Cluster.LendingFileValues,
		LendingSettingsStore:      offloadsettings.NewStore(stores.handle.DB(), stores.handle.Reader()),
		OffloadDecisionStore:      stores.offloadDecisions,
		AnalyticsStore:            stores.analytics,
		RouterStore:               stores.handle,
		LoadCaptureStore:          stores.loadCaptures,
		LoadErrorStore:            stores.loadErrors,
		LoadCaptureMaxOutputBytes: cfg.Analytics.LoadCaptureMaxOutputMB * 1024 * 1024,
		VRAMAnalyticsEnabled:      cfg.Analytics.Enabled && cfg.Analytics.VRAMEnabled,
		VRAMSampleInterval:        cfg.Analytics.VRAMSampleInterval,
		MemorySource:              hardware.NewNodeMemorySource(),
		Downloader:                runtime.downloader,
		DownloaderCapability:      runtime.downloaderCapability,
		VLLM:                      runtime.vllm,
		VLLMUnavailableReason:     runtime.vllmUnavailableReason,
		VLLMDynamicLoRAEnabled:    cfg.VLLM.DynamicLoRAEnabled,
		VLLMEEPEnabled:            cfg.VLLM.EEPEnabled,
		FFmpeg:                    runtime.ffmpeg,
		FFmpegScratchDir:          cfg.FFmpeg.ScratchDir,
		Logger:                    runtime.logger,
		Shutdown:                  routerShutdownFunc(cfg, shutdownRequested),
		TransportLimits: transportbody.Limits{
			ReplayBufferBytes: cfg.Limits.ReplayBufferMB * transportbody.MiB,
			MemoryBudgetBytes: cfg.Limits.MemoryBudgetMB * transportbody.MiB,
			MaxRequestBytes:   cfg.Limits.MaxStreamRequestGB * transportbody.GiB,
			MaxResponseBytes:  cfg.Limits.MaxStreamResponseGB * transportbody.GiB,
			SelectorScanBytes: cfg.Limits.SelectorScanMB * transportbody.MiB,
		},
		MaxControlBodyBytes:  cfg.Limits.MaxControlBodyMB * transportbody.MiB,
		SeparateRuntimeLimit: cfg.Limits.SeparateRuntimes,
	}
}
