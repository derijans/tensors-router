package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"tensors-router/internal/downloader"
)

func optionalDownloader(routerConfigPath string, logger *log.Logger) (*downloader.Manager, downloader.Capability) {
	executablePath, err := os.Executable()
	if err != nil {
		return nil, downloader.Capability{Error: err.Error()}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(executablePath), downloaderExecutableName())); err != nil {
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
