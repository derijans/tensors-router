package config

import (
	"fmt"

	"tensors-router/internal/flatyaml"
	"tensors-router/internal/offloadsettings"
)

func (cfg *Config) schema() flatyaml.Schema {
	return flatyaml.Schema{
		"security": {Fields: flatyaml.Fields{
			"profile": flatyaml.String(&cfg.Security.Profile),
		}},
		"server": {Fields: flatyaml.Fields{
			"bind":          flatyaml.String(&cfg.Server.Bind),
			"allowed_cidrs": flatyaml.StringList(&cfg.Server.AllowedCIDRs),
		}},
		"auth": {Fields: flatyaml.Fields{
			"inference_keys": flatyaml.StringList(&cfg.Auth.InferenceKeys),
			"admin_keys":     flatyaml.StringList(&cfg.Auth.AdminKeys),
			"bearer_keys":    flatyaml.StringList(&cfg.Auth.BearerKeys),
		}},
		"models": {Fields: flatyaml.Fields{
			"config_dir":                 flatyaml.String(&cfg.Models.ConfigDir),
			"startup_model":              flatyaml.String(&cfg.Models.StartupModel),
			"shared_dir":                 flatyaml.String(&cfg.Models.SharedDir),
			"hash_workers":               flatyaml.Int(&cfg.Models.HashWorkers),
			"concurrent_asset_transfers": flatyaml.Int(&cfg.Models.ConcurrentAssetTransfers),
			"file_roots":                 flatyaml.StringList(&cfg.Models.FileRoots),
		}},
		"mcp": {Fields: flatyaml.Fields{
			"enabled":   flatyaml.Bool(&cfg.MCP.Enabled),
			"directory": flatyaml.String(&cfg.MCP.Directory),
		}},
		"ffmpeg": {Fields: flatyaml.Fields{
			"binary_path": flatyaml.String(&cfg.FFmpeg.BinaryPath),
			"scratch_dir": flatyaml.String(&cfg.FFmpeg.ScratchDir),
		}},
		"backend": {Fields: flatyaml.Fields{
			"mode": flatyaml.String(&cfg.Backend.Mode),
		}},
		"kobold":      {Fields: koboldFields(&cfg.Kobold)},
		"llama":       {Fields: nativeServerFields(&cfg.Llama, true)},
		"sdcpp":       {Fields: nativeServerFields(&cfg.SDCPP, false)},
		"whispercpp":  {Fields: nativeServerFields(&cfg.WhisperCPP, false)},
		"vllm":        {Fields: vllmFields(&cfg.VLLM)},
		"logging":     {Fields: loggingFields(&cfg.Logging)},
		"updates":     {Fields: updatesFields(&cfg.Updates)},
		"downloader":  {Fields: downloaderFields(&cfg.Downloader)},
		"cluster":     {Fields: clusterFields(&cfg.Cluster), Dynamic: lendingField(cfg.Cluster.LendingFileValues)},
		"analytics":   {Fields: analyticsFields(&cfg.Analytics)},
		"diagnostics": {Fields: diagnosticsFields(&cfg.Diagnostics)},
		"limits":      {Fields: limitsFields(&cfg.Limits)},
	}
}

func koboldFields(kobold *KoboldConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"backend_url":            flatyaml.String(&kobold.BackendURL),
		"embeddings_backend_url": flatyaml.String(&kobold.EmbeddingsBackendURL).Marking(&kobold.embeddingsBackendURLSet),
		"binary_path":            flatyaml.String(&kobold.BinaryPath),
		"data_dir":               flatyaml.String(&kobold.DataDir),
		"multiuser":              flatyaml.Int(&kobold.Multiuser),
		"quiet":                  flatyaml.Bool(&kobold.Quiet),
		"skip_launcher":          flatyaml.Bool(&kobold.SkipLauncher),
		"no_model":               flatyaml.Bool(&kobold.NoModel),
		"hide_window":            flatyaml.Bool(&kobold.HideWindow),
		"extra_args":             flatyaml.StringList(&kobold.ExtraArgs),
	}
}

func nativeServerFields(server *NativeServerConfig, servesEmbeddings bool) flatyaml.Fields {
	fields := flatyaml.Fields{
		"backend_url": flatyaml.String(&server.BackendURL),
		"binary_path": flatyaml.String(&server.BinaryPath),
		"data_dir":    flatyaml.String(&server.DataDir),
		"hide_window": flatyaml.Bool(&server.HideWindow),
		"extra_args":  flatyaml.StringList(&server.ExtraArgs),
	}
	if servesEmbeddings {
		fields["embeddings_backend_url"] = flatyaml.String(&server.EmbeddingsBackendURL).Marking(&server.embeddingsBackendURLSet)
	}
	return fields
}

func vllmFields(vllm *VLLMConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"binary_location":            flatyaml.String(&vllm.BinaryLocation),
		"data_dir":                   flatyaml.String(&vllm.DataDir),
		"profile":                    flatyaml.String(&vllm.Profile),
		"manifest_path":              flatyaml.String(&vllm.ManifestPath),
		"manifest_sha256":            flatyaml.String(&vllm.ManifestSHA256),
		"manifest_size":              flatyaml.Int64(&vllm.ManifestSize),
		"tuf_repository_url":         flatyaml.String(&vllm.TUFRepositoryURL),
		"tuf_root_path":              flatyaml.String(&vllm.TUFRootPath),
		"dynamic_lora_enabled":       flatyaml.Bool(&vllm.DynamicLoRAEnabled),
		"eep_enabled":                flatyaml.Bool(&vllm.EEPEnabled),
		"trust_remote_code":          flatyaml.Bool(&vllm.TrustRemoteCode),
		"external_tools":             flatyaml.Bool(&vllm.ExternalTools),
		"oci_run_as_image_user":      flatyaml.Bool(&vllm.OCIRunAsImageUser),
		"allow_unverified_install":   flatyaml.Bool(&vllm.AllowUnverifiedInstall),
		"unverified_vllm_version":    flatyaml.String(&vllm.UnverifiedVLLMVersion),
		"unverified_python_version":  flatyaml.String(&vllm.UnverifiedPythonVersion),
		"unverified_index_url":       flatyaml.String(&vllm.UnverifiedIndexURL),
		"unverified_extra_index_url": flatyaml.String(&vllm.UnverifiedExtraIndexURL),
	}
}

func loggingFields(logging *LoggingConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"mode":                 flatyaml.String(&logging.Mode).Marking(&logging.modeSet),
		"enabled":              flatyaml.Bool(&logging.Enabled).Marking(&logging.legacyEnabledSet),
		"backend_logs_to_disk": flatyaml.Bool(&logging.BackendLogsToDisk),
	}
}

func updatesFields(updates *UpdatesConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"enabled":                   flatyaml.Bool(&updates.Enabled),
		"check_interval":            flatyaml.Duration(&updates.CheckInterval),
		"include_prereleases":       flatyaml.Bool(&updates.IncludePrereleases),
		"tuf_repository_url":        flatyaml.String(&updates.TUFRepositoryURL),
		"tuf_root_path":             flatyaml.String(&updates.TUFRootPath),
		"binary_url":                flatyaml.String(&updates.BinaryURL),
		"binary_sha256":             flatyaml.String(&updates.BinarySHA256),
		"binary_repository_url":     flatyaml.String(&updates.BinaryRepositoryURL),
		"binary_asset_glob":         flatyaml.String(&updates.BinaryAssetGlob),
		"llama_binary_url":          flatyaml.String(&updates.LlamaBinaryURL),
		"llama_binary_sha256":       flatyaml.String(&updates.LlamaSHA256),
		"llama_repository_url":      flatyaml.String(&updates.LlamaRepositoryURL),
		"llama_asset_glob":          flatyaml.String(&updates.LlamaAssetGlob),
		"sdcpp_binary_url":          flatyaml.String(&updates.SDCPPBinaryURL),
		"sdcpp_binary_sha256":       flatyaml.String(&updates.SDCPPSHA256),
		"sdcpp_repository_url":      flatyaml.String(&updates.SDCPPRepositoryURL),
		"sdcpp_asset_glob":          flatyaml.String(&updates.SDCPPAssetGlob),
		"whispercpp_binary_url":     flatyaml.String(&updates.WhisperCPPBinaryURL),
		"whispercpp_binary_sha256":  flatyaml.String(&updates.WhisperCPPSHA256),
		"whispercpp_repository_url": flatyaml.String(&updates.WhisperCPPRepositoryURL),
		"whispercpp_asset_glob":     flatyaml.String(&updates.WhisperCPPAssetGlob),
	}
}

func downloaderFields(downloader *DownloaderConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"enabled":         flatyaml.Bool(&downloader.Enabled),
		"binary_location": flatyaml.String(&downloader.BinaryLocation),
		"config_path":     flatyaml.String(&downloader.ConfigPath),
	}
}

func clusterFields(cluster *ClusterConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"role":             flatyaml.String(&cluster.Role),
		"node_id":          flatyaml.String(&cluster.NodeID),
		"public_url":       flatyaml.String(&cluster.PublicURL),
		"master_url":       flatyaml.String(&cluster.MasterURL),
		"token":            flatyaml.String(&cluster.Token),
		"store_dir":        flatyaml.String(&cluster.StoreDir),
		"database_path":    flatyaml.String(&cluster.DatabasePath),
		"sync_interval":    flatyaml.Duration(&cluster.SyncInterval),
		"health_interval":  flatyaml.Duration(&cluster.HealthInterval),
		"control_timeout":  flatyaml.Duration(&cluster.ControlTimeout),
		"sync_concurrency": flatyaml.Int(&cluster.SyncConcurrency),
		"slave_urls":       flatyaml.StringList(&cluster.SlaveURLs),
	}
}

func lendingField(values offloadsettings.Values) func(key string) (flatyaml.Field, bool) {
	return func(key string) (flatyaml.Field, bool) {
		if !offloadsettings.Known(key) {
			return flatyaml.Field{}, false
		}
		return flatyaml.Scalar(func(value string) error {
			normalized, err := offloadsettings.Normalize(key, value)
			if err != nil {
				return fmt.Errorf("cluster.%w", err)
			}
			values[key] = normalized
			return nil
		}), true
	}
}

func analyticsFields(analytics *AnalyticsConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"enabled":                    flatyaml.Bool(&analytics.Enabled),
		"vram_enabled":               flatyaml.Bool(&analytics.VRAMEnabled),
		"load_capture_enabled":       flatyaml.Bool(&analytics.LoadCaptureEnabled),
		"load_capture_database_path": flatyaml.String(&analytics.LoadCaptureDatabasePath),
		"load_capture_max_output_mb": flatyaml.Int64(&analytics.LoadCaptureMaxOutputMB),
		"flush_interval":             flatyaml.Duration(&analytics.FlushInterval),
		"database_path":              flatyaml.String(&analytics.DatabasePath),
		"raw_retention":              flatyaml.Duration(&analytics.RawRetention),
		"vram_sample_interval":       flatyaml.Duration(&analytics.VRAMSampleInterval),
	}
}

func diagnosticsFields(diagnostics *DiagnosticsConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"enabled":       flatyaml.Bool(&diagnostics.Enabled),
		"database_path": flatyaml.String(&diagnostics.DatabasePath),
		"retention":     flatyaml.Duration(&diagnostics.Retention),
		"max_output_kb": flatyaml.Int64(&diagnostics.MaxOutputKB),
	}
}

func limitsFields(limits *LimitsConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"max_control_body_mb":    flatyaml.Int64(&limits.MaxControlBodyMB),
		"replay_buffer_mb":       flatyaml.Int64(&limits.ReplayBufferMB),
		"memory_budget_mb":       flatyaml.Int64(&limits.MemoryBudgetMB),
		"max_stream_request_gb":  flatyaml.Int64(&limits.MaxStreamRequestGB),
		"max_stream_response_gb": flatyaml.Int64(&limits.MaxStreamResponseGB),
		"selector_scan_mb":       flatyaml.Int64(&limits.SelectorScanMB),
		"separate_runtimes":      flatyaml.Int(&limits.SeparateRuntimes),
		"drain_timeout":          flatyaml.Duration(&limits.DrainTimeout),
	}
}
