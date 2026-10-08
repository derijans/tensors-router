package config

import (
	"runtime"
	"time"

	"tensors-router/internal/offloadsettings"
)

func Defaults() Config {
	return Config{
		Security: SecurityConfig{
			Profile: SecurityProfileSecure,
		},
		Server: ServerConfig{
			Bind: "127.0.0.1:8080",
			AllowedCIDRs: []string{
				"127.0.0.0/8",
				"::1/128",
				"10.0.0.0/8",
				"172.16.0.0/12",
				"192.168.0.0/16",
			},
		},
		Auth: AuthConfig{
			InferenceKeys: []string{},
			AdminKeys:     []string{},
			BearerKeys:    []string{},
		},
		Models: ModelsConfig{
			ConfigDir:                "./kcpps",
			FileRoots:                []string{},
			HashWorkers:              1,
			ConcurrentAssetTransfers: 2,
		},
		Backend: BackendConfig{
			Mode: "kobold",
		},
		Kobold: KoboldConfig{
			BackendURL:           "http://127.0.0.1:5001",
			EmbeddingsBackendURL: "http://127.0.0.1:0",
			BinaryPath:           "./bin/kobold/koboldcpp",
			DataDir:              "./data",
			Multiuser:            1,
			ExtraArgs:            []string{},
			Quiet:                true,
			SkipLauncher:         true,
			NoModel:              true,
			HideWindow:           true,
		},
		Llama: NativeServerConfig{
			BackendURL:           "http://127.0.0.1:5002",
			EmbeddingsBackendURL: "http://127.0.0.1:0",
			BinaryPath:           "./bin/llama/llama-server",
			DataDir:              "./data/llama",
			ExtraArgs:            []string{},
			HideWindow:           true,
		},
		SDCPP: NativeServerConfig{
			BackendURL: "http://127.0.0.1:7860",
			BinaryPath: "./bin/stable-diffusion/build/bin/sd-server",
			DataDir:    "./data/sdcpp",
			ExtraArgs:  []string{},
			HideWindow: true,
		},
		WhisperCPP: NativeServerConfig{
			BackendURL: "http://127.0.0.1:5003",
			BinaryPath: "./bin/whisper/whisper-server",
			DataDir:    "./data/whispercpp",
			ExtraArgs:  []string{},
			HideWindow: true,
		},
		VLLM: VLLMConfig{
			DataDir:          "./data/vllm",
			Profile:          "auto",
			ManifestPath:     "runtimes/vllm/" + runtime.GOOS + "-" + runtime.GOARCH + ".json",
			TUFRepositoryURL: "https://derijans.github.io/tensors-router/tuf/metadata",
		},
		Logging: LoggingConfig{
			Mode:              LoggingModeNormal,
			Enabled:           true,
			BackendLogsToDisk: false,
		},
		Updates: UpdatesConfig{
			Enabled:                 false,
			CheckInterval:           168 * time.Hour,
			TUFRepositoryURL:        "https://derijans.github.io/tensors-router/tuf/metadata",
			BinaryURL:               "",
			BinarySHA256:            "",
			BinaryRepositoryURL:     "https://github.com/LostRuins/koboldcpp",
			LlamaBinaryURL:          "",
			LlamaSHA256:             "",
			SDCPPBinaryURL:          "",
			SDCPPSHA256:             "",
			LlamaRepositoryURL:      "https://github.com/ggml-org/llama.cpp",
			SDCPPRepositoryURL:      "https://github.com/leejet/stable-diffusion.cpp",
			WhisperCPPRepositoryURL: "https://github.com/ggml-org/whisper.cpp",
		},
		Downloader: DownloaderConfig{
			Enabled: true,
		},
		Cluster: ClusterConfig{
			Role:              "standalone",
			NodeID:            "local",
			SlaveURLs:         []string{},
			StoreDir:          "./router-store",
			SyncInterval:      60 * time.Second,
			HealthInterval:    15 * time.Second,
			ControlTimeout:    30 * time.Second,
			SyncConcurrency:   4,
			LendingFileValues: offloadsettings.Values{},
		},
		Analytics: AnalyticsConfig{
			Enabled:                false,
			VRAMEnabled:            true,
			LoadCaptureEnabled:     false,
			LoadCaptureMaxOutputMB: 64,
			FlushInterval:          3 * time.Minute,
			RawRetention:           30 * 24 * time.Hour,
			VRAMSampleInterval:     time.Second,
		},
		Diagnostics: DiagnosticsConfig{
			Enabled:     true,
			Retention:   30 * 24 * time.Hour,
			MaxOutputKB: 64,
		},
		Limits: LimitsConfig{
			MaxControlBodyMB:    8,
			ReplayBufferMB:      64,
			MemoryBudgetMB:      2048,
			MaxStreamRequestGB:  32,
			MaxStreamResponseGB: 32,
			SelectorScanMB:      64,
			SeparateRuntimes:    5,
			DrainTimeout:        15 * time.Minute,
		},
		MCP: MCPConfig{
			Enabled:   false,
			Directory: "./mcp",
		},
	}
}
