package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"tensors-router/internal/companion"
	"tensors-router/internal/config"
	"tensors-router/internal/downloader"
)

func optionalDownloader(routerConfigPath string, downloaderConfig config.DownloaderConfig, logger *log.Logger) (manager *downloader.Manager, capability downloader.Capability) {
	capability.Enabled = downloaderConfig.Enabled
	defer func() { logDownloaderStatus(logger, capability) }()
	if !downloaderConfig.Enabled {
		return nil, failedDownloaderCapability(capability, "disabled by configuration")
	}
	binaryPath, err := downloaderBinaryPath(routerConfigPath, downloaderConfig.BinaryLocation)
	if err != nil {
		return nil, failedDownloaderCapability(capability, fmt.Sprintf("detect downloader companion: %v", err))
	}
	binaryInfo, err := os.Stat(binaryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, failedDownloaderCapability(capability, fmt.Sprintf("downloader companion not found at %q", binaryPath))
		}
		return nil, failedDownloaderCapability(capability, fmt.Sprintf("inspect downloader companion %q: %v", binaryPath, err))
	}
	if !binaryInfo.Mode().IsRegular() {
		return nil, failedDownloaderCapability(capability, fmt.Sprintf("downloader companion at %q is not a regular file", binaryPath))
	}
	capability.Present = true
	capability.Available = true
	configPath := filepath.Join(filepath.Dir(routerConfigPath), "downloader.yaml")
	config, warnings, err := downloader.LoadConfig(configPath)
	if err != nil {
		return nil, failedDownloaderCapability(capability, fmt.Sprintf("load downloader configuration %q: %v", configPath, err))
	}
	for _, warning := range warnings {
		logger.Printf("configuration warning: %s", warning)
	}
	manager, err = downloader.NewManager(config, "")
	if err != nil {
		return nil, failedDownloaderCapability(capability, err.Error())
	}
	capability.Working = true
	capability = downloader.MergeRuntimeCapability(capability, manager.Capability())
	if !capability.Working {
		_ = manager.Close()
		return nil, capability
	}
	return manager, capability
}

func failedDownloaderCapability(capability downloader.Capability, reason string) downloader.Capability {
	capability.Working = false
	capability.Reason = reason
	capability.Error = reason
	return capability
}

func logDownloaderStatus(logger *log.Logger, capability downloader.Capability) {
	if capability.Working {
		logger.Printf("downloader status enabled=%t present=%t working=%t", capability.Enabled, capability.Present, capability.Working)
		return
	}
	logger.Printf("downloader status enabled=%t present=%t working=%t reason=%q", capability.Enabled, capability.Present, capability.Working, capability.Reason)
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
