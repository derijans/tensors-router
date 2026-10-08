package main

import (
	"context"
	"path/filepath"

	"tensors-router/internal/config"
	"tensors-router/internal/kobold"
	"tensors-router/internal/mcp"
	"tensors-router/internal/native"
	"tensors-router/internal/proxy"
)

func createBackends(cfg config.Config, mcpReconciler *mcp.Reconciler, videoFFmpegDir string) (map[string]proxy.BackendFamilyConfig, []func(context.Context) error, error) {
	koboldManager, err := kobold.NewManager(koboldProcessConfig(cfg, mcpReconciler))
	if err != nil {
		return nil, nil, err
	}

	llamaConfig := llamaProcessConfig(cfg, mcpReconciler)
	llamaConfig.VideoFFmpegDir = videoFFmpegDir
	llamaManager, err := native.NewLlamaManager(llamaConfig)
	if err != nil {
		return nil, nil, err
	}
	sdcppManager, err := native.NewSDCPPManager(sdcppProcessConfig(cfg))
	if err != nil {
		return nil, nil, err
	}
	whisperCPPManager, err := native.NewWhisperCPPManager(whisperCPPProcessConfig(cfg))
	if err != nil {
		return nil, nil, err
	}

	families := map[string]proxy.BackendFamilyConfig{
		proxy.BackendModeKobold: {
			TextBackend:     koboldManager,
			ImageBackend:    koboldManager,
			SeparateBackend: koboldSeparateBackend(cfg),
			Start:           koboldManager.Start,
			Stop:            stopKoboldManagers(koboldManager),
			StopPrimary:     koboldManager.Stop,
		},
		proxy.BackendModeLlamaSDCPP: {
			TextBackend:          llamaManager,
			ImageBackend:         sdcppManager,
			TranscriptionBackend: whisperCPPManager,
			SeparateBackend:      llamaSeparateBackend(cfg, videoFFmpegDir),
			Stop:                 stopNativeManagers(llamaManager, sdcppManager, whisperCPPManager),
			StopPrimary:          stopNativeManagers(llamaManager, sdcppManager, whisperCPPManager),
		},
	}
	shutdownBackends := []func(context.Context) error{
		stopKoboldManagers(koboldManager),
		stopNativeManagers(llamaManager, sdcppManager, whisperCPPManager),
		releaseKoboldEndpoints(koboldManager),
		releaseNativeEndpoints(llamaManager, sdcppManager, whisperCPPManager),
	}
	return families, shutdownBackends, nil
}

func koboldSeparateBackend(cfg config.Config) func(string, string) (proxy.Backend, error) {
	return func(name string, lane string) (proxy.Backend, error) {
		processConfig := koboldProcessConfig(cfg)
		processConfig.BackendURL = "http://127.0.0.1:0"
		processConfig.DataDir = filepath.Join(cfg.Kobold.DataDir, "separate", name)
		processConfig.MCP = nil
		if lane == "embeddings" {
			return kobold.NewEmbeddingsManager(processConfig)
		}
		return kobold.NewManager(processConfig)
	}
}

func llamaSeparateBackend(cfg config.Config, videoFFmpegDir string) func(string, string) (proxy.Backend, error) {
	return func(name string, lane string) (proxy.Backend, error) {
		processConfig := llamaProcessConfig(cfg)
		processConfig.VideoFFmpegDir = videoFFmpegDir
		processConfig.BackendURL = "http://127.0.0.1:0"
		processConfig.DataDir = filepath.Join(cfg.Llama.DataDir, "separate", name)
		processConfig.MCP = nil
		if lane == "embeddings" {
			return native.NewLlamaEmbeddingsManager(processConfig)
		}
		return native.NewLlamaManager(processConfig)
	}
}

func koboldProcessConfig(cfg config.Config, reconciler ...*mcp.Reconciler) kobold.ProcessConfig {
	mcpReconciler := firstMCPReconciler(reconciler)
	return kobold.ProcessConfig{
		BackendURL:   cfg.Kobold.BackendURL,
		BinaryPath:   cfg.Kobold.BinaryPath,
		ConfigDir:    cfg.Models.ConfigDir,
		DataDir:      cfg.Kobold.DataDir,
		ExtraArgs:    cfg.Kobold.ExtraArgs,
		Multiuser:    cfg.Kobold.Multiuser,
		Quiet:        cfg.Kobold.Quiet,
		SkipLauncher: cfg.Kobold.SkipLauncher,
		NoModel:      cfg.Kobold.NoModel,
		HideWindow:   cfg.Kobold.HideWindow,
		Logging:      cfg.Logging.BackendLogsToDisk,
		MCP:          mcpReconciler,
	}
}

func llamaProcessConfig(cfg config.Config, reconciler ...*mcp.Reconciler) native.ProcessConfig {
	mcpReconciler := firstMCPReconciler(reconciler)
	return native.ProcessConfig{
		BackendURL: cfg.Llama.BackendURL,
		BinaryPath: cfg.Llama.BinaryPath,
		ConfigDir:  cfg.Models.ConfigDir,
		DataDir:    cfg.Llama.DataDir,
		ExtraArgs:  cfg.Llama.ExtraArgs,
		HideWindow: cfg.Llama.HideWindow,
		Logging:    cfg.Logging.BackendLogsToDisk,
		MCP:        mcpReconciler,
	}
}

func stopKoboldManagers(managers ...*kobold.Manager) func(context.Context) error {
	return func(ctx context.Context) error {
		var firstErr error
		for _, manager := range managers {
			if err := manager.Stop(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
}

// releaseKoboldEndpoints returns dynamically allocated ports to the shared
// allocator. This is distinct from stopKoboldManagers, which also backs the
// family's runtime Stop/StopPrimary hooks (invoked while the router keeps
// running, e.g. from an admin endpoint) — releasing the port there would
// break the sticky-port guarantee across a restart. Call this only from the
// final process shutdown path.
func releaseKoboldEndpoints(managers ...*kobold.Manager) func(context.Context) error {
	return func(context.Context) error {
		for _, manager := range managers {
			manager.ReleaseEndpoint()
		}
		return nil
	}
}

func firstMCPReconciler(values []*mcp.Reconciler) *mcp.Reconciler {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

func sdcppProcessConfig(cfg config.Config) native.ProcessConfig {
	return native.ProcessConfig{
		BackendURL: cfg.SDCPP.BackendURL,
		BinaryPath: cfg.SDCPP.BinaryPath,
		ConfigDir:  cfg.Models.ConfigDir,
		DataDir:    cfg.SDCPP.DataDir,
		ExtraArgs:  cfg.SDCPP.ExtraArgs,
		HideWindow: cfg.SDCPP.HideWindow,
		Logging:    cfg.Logging.BackendLogsToDisk,
	}
}

func whisperCPPProcessConfig(cfg config.Config) native.ProcessConfig {
	return native.ProcessConfig{
		BackendURL: cfg.WhisperCPP.BackendURL,
		BinaryPath: cfg.WhisperCPP.BinaryPath,
		ConfigDir:  cfg.Models.ConfigDir,
		DataDir:    cfg.WhisperCPP.DataDir,
		ExtraArgs:  cfg.WhisperCPP.ExtraArgs,
		HideWindow: cfg.WhisperCPP.HideWindow,
		Logging:    cfg.Logging.BackendLogsToDisk,
	}
}

func stopNativeManagers(managers ...*native.Manager) func(context.Context) error {
	return func(ctx context.Context) error {
		var firstErr error
		for _, manager := range managers {
			if err := manager.Unload(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
}

// releaseNativeEndpoints returns dynamically allocated ports to the shared
// allocator. This is distinct from stopNativeManagers, which also backs the
// family's runtime Stop/StopPrimary hooks (invoked while the router keeps
// running, e.g. from an admin endpoint) — releasing the port there would
// break the sticky-port guarantee across a reload. Call this only from the
// final process shutdown path.
func releaseNativeEndpoints(managers ...*native.Manager) func(context.Context) error {
	return func(context.Context) error {
		for _, manager := range managers {
			manager.ReleaseEndpoint()
		}
		return nil
	}
}
