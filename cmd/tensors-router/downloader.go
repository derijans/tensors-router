package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"tensors-router/internal/companion"
	"tensors-router/internal/config"
	"tensors-router/internal/downloader"
)

func optionalDownloader(routerConfigPath string, downloaderConfig config.DownloaderConfig, logger *log.Logger) (client downloader.Service, capability downloader.Capability) {
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
	configPath, err := downloaderConfigPath(routerConfigPath, downloaderConfig.ConfigPath)
	if err != nil {
		return nil, failedDownloaderCapability(capability, fmt.Sprintf("resolve downloader configuration path: %v", err))
	}
	_, warnings, err := downloader.LoadConfig(configPath)
	if err != nil {
		return nil, failedDownloaderCapability(capability, fmt.Sprintf("load downloader configuration %q: %v", configPath, err))
	}
	for _, warning := range warnings {
		logger.Printf("configuration warning: %s", warning)
	}
	client, err = downloader.StartSupervisedClient(context.Background(), binaryPath, configPath)
	if err != nil {
		return nil, failedDownloaderCapability(capability, err.Error())
	}
	capability.Working = true
	capability = downloader.MergeRuntimeCapability(capability, client.Capability())
	if !capability.Working {
		_ = client.Close()
		return nil, capability
	}
	return client, capability
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

func downloaderConfigPath(routerConfigPath string, configured string) (string, error) {
	if configured == "" {
		configured = "downloader.yaml"
	}
	if !filepath.IsAbs(configured) {
		configured = filepath.Join(filepath.Dir(routerConfigPath), configured)
	}
	return filepath.Abs(configured)
}

func downloaderBinaryPath(routerConfigPath string, binaryLocation string) (string, error) {
	if binaryLocation != "" {
		if filepath.IsAbs(binaryLocation) {
			return binaryLocation, nil
		}
		return filepath.Abs(filepath.Join(filepath.Dir(routerConfigPath), binaryLocation))
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

func closeDownloader(service downloader.Service) error {
	if service == nil {
		return nil
	}
	return service.Close()
}

func modelFileRoots(configured []string, capability downloader.Capability) []string {
	roots := append([]string{}, configured...)
	storageRoot := strings.TrimSpace(capability.StorageRoot)
	if !capability.Working || storageRoot == "" || fileSystemRoot(storageRoot) {
		return roots
	}
	for _, root := range roots {
		if sameOrParentDirectory(root, storageRoot) {
			return roots
		}
	}
	return append(roots, storageRoot)
}

func fileSystemRoot(path string) bool {
	cleaned := filepath.Clean(path)
	return filepath.Dir(cleaned) == cleaned
}

func sameOrParentDirectory(parent string, child string) bool {
	parentPath, parentErr := filepath.Abs(parent)
	childPath, childErr := filepath.Abs(child)
	if parentErr != nil || childErr != nil {
		return false
	}
	relative, err := filepath.Rel(parentPath, childPath)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
