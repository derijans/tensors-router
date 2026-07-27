package main

import (
	"io"
	"log"
	"path/filepath"
	"testing"

	"tensors-router/internal/config"
)

func TestDownloaderBinaryPathUsesConfiguredLocation(t *testing.T) {
	routerConfigPath := filepath.Join("configs", "router.yaml")

	path, err := downloaderBinaryPath(routerConfigPath, filepath.Join("tools", "tensor-router-downloader"))
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join("configs", "tools", "tensor-router-downloader") {
		t.Fatalf("unexpected relative binary path %q", path)
	}

	absolutePath := filepath.Join(t.TempDir(), "tensor-router-downloader")
	path, err = downloaderBinaryPath(routerConfigPath, absolutePath)
	if err != nil {
		t.Fatal(err)
	}
	if path != absolutePath {
		t.Fatalf("unexpected absolute binary path %q", path)
	}
}

func TestOptionalDownloaderRespectsDisabledConfig(t *testing.T) {
	manager, capability := optionalDownloader(filepath.Join(t.TempDir(), "router.yaml"), config.DownloaderConfig{Enabled: false}, log.New(io.Discard, "", 0))
	if manager != nil || capability.Available || capability.Error != "" {
		t.Fatalf("unexpected disabled downloader result %#v %#v", manager, capability)
	}
}
