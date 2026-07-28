package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"tensors-router/internal/companion"
	"tensors-router/internal/config"
	"tensors-router/internal/downloader"
)

func optionalDownloader(routerConfigPath string, downloaderConfig config.DownloaderConfig, logger *log.Logger) (*downloader.Manager, downloader.Capability) {
	if !downloaderConfig.Enabled {
		return nil, downloader.Capability{}
	}
	binaryPath, err := downloaderBinaryPath(routerConfigPath, downloaderConfig.BinaryLocation)
	if err != nil {
		return nil, downloader.Capability{Error: err.Error()}
	}
	if _, err := os.Stat(binaryPath); err != nil {
		if os.IsNotExist(err) {
			return nil, downloader.Capability{}
		}
		return nil, downloader.Capability{Error: err.Error()}
	}
	configPath := filepath.Join(filepath.Dir(routerConfigPath), "downloader.yaml")
	config, warnings, err := downloader.LoadConfig(configPath)
	if err != nil {
		return nil, downloader.Capability{Available: true, Error: err.Error()}
	}
	for _, warning := range warnings {
		logger.Printf("configuration warning: %s", warning)
	}
	manager, err := downloader.NewManager(config, "")
	if err != nil {
		return nil, downloader.Capability{Available: true, Error: err.Error()}
	}
	return manager, manager.Capability()
}

func downloaderBinaryPath(routerConfigPath string, binaryLocation string) (string, error) {
	if binaryLocation != "" {
		if filepath.IsAbs(binaryLocation) {
			return binaryLocation, nil
		}
		return filepath.Join(filepath.Dir(routerConfigPath), binaryLocation), nil
	}
	executablePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if path, found := companion.FindSibling(executablePath, "tensor-router-downloader", "tensors-router"); found {
		return path, nil
	}
	return companion.PreferredSibling(executablePath, "tensor-router-downloader", "tensors-router"), nil
}

func closeDownloader(manager *downloader.Manager) error {
	if manager == nil {
		return nil
	}
	return manager.Close()
}

func downloaderExecutableName() string {
	if runtime.GOOS == "windows" {
		return "tensor-router-downloader.exe"
	}
	return "tensor-router-downloader"
}
