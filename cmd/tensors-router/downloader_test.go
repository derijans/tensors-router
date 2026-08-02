package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	var output bytes.Buffer
	manager, capability := optionalDownloader(filepath.Join(t.TempDir(), "router.yaml"), config.DownloaderConfig{Enabled: false}, log.New(&output, "", 0))
	if manager != nil || capability.Enabled || capability.Present || capability.Working || capability.Reason != "disabled by configuration" {
		t.Fatalf("unexpected disabled downloader result %#v %#v", manager, capability)
	}
	assertDownloaderStatusLog(t, output.String(), false, false, false, true)
}

func TestOptionalDownloaderReportsMissingCompanion(t *testing.T) {
	directory := t.TempDir()
	var output bytes.Buffer
	manager, capability := optionalDownloader(filepath.Join(directory, "router.yaml"), config.DownloaderConfig{Enabled: true, BinaryLocation: "missing-downloader"}, log.New(&output, "", 0))
	if manager != nil || !capability.Enabled || capability.Present || capability.Working || !strings.Contains(capability.Reason, "companion not found") {
		t.Fatalf("unexpected missing companion status %#v", capability)
	}
	assertDownloaderStatusLog(t, output.String(), true, false, false, true)
}

func TestOptionalDownloaderReportsMissingAndInvalidConfiguration(t *testing.T) {
	for _, test := range []struct {
		name          string
		configuration *string
		reason        string
	}{
		{name: "missing", reason: "load downloader configuration"},
		{name: "invalid", configuration: stringPointer("invalid"), reason: "expected a section"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeDownloaderCompanion(t, directory)
			if test.configuration != nil {
				if err := os.WriteFile(filepath.Join(directory, "downloader.yaml"), []byte(*test.configuration), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			manager, capability := optionalDownloader(filepath.Join(directory, "router.yaml"), config.DownloaderConfig{Enabled: true, BinaryLocation: "downloader-companion"}, log.New(io.Discard, "", 0))
			if manager != nil || !capability.Enabled || !capability.Present || capability.Working || !strings.Contains(capability.Reason, test.reason) {
				t.Fatalf("unexpected configuration failure status %#v", capability)
			}
		})
	}
}

func TestOptionalDownloaderReportsStorageAndDatabaseInitializationFailures(t *testing.T) {
	for _, test := range []struct {
		name          string
		configuration string
		prepare       func(*testing.T, string)
		reason        string
	}{
		{name: "storage", prepare: func(t *testing.T, directory string) {
			if err := os.WriteFile(filepath.Join(directory, "models"), []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, reason: "initialize downloader storage root"},
		{name: "database", configuration: "storage:\n  database_path: ./downloader-state\n", prepare: func(*testing.T, string) {}, reason: "initialize downloader database"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeDownloaderCompanion(t, directory)
			if err := os.WriteFile(filepath.Join(directory, "downloader.yaml"), []byte(test.configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			test.prepare(t, directory)
			manager, capability := optionalDownloader(filepath.Join(directory, "router.yaml"), config.DownloaderConfig{Enabled: true, BinaryLocation: "downloader-companion"}, log.New(io.Discard, "", 0))
			if manager != nil || capability.Working || !strings.Contains(capability.Reason, test.reason) {
				t.Fatalf("unexpected initialization failure status %#v", capability)
			}
		})
	}
}

func TestOptionalDownloaderReportsSuccessfulReadiness(t *testing.T) {
	directory := t.TempDir()
	writeDownloaderCompanion(t, directory)
	if err := os.WriteFile(filepath.Join(directory, "downloader.yaml"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	manager, capability := optionalDownloader(filepath.Join(directory, "router.yaml"), config.DownloaderConfig{Enabled: true, BinaryLocation: "downloader-companion"}, log.New(&output, "", 0))
	if manager == nil || !capability.Enabled || !capability.Present || !capability.Working || capability.Reason != "" || capability.Error != "" {
		t.Fatalf("unexpected ready status %#v", capability)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	assertDownloaderStatusLog(t, output.String(), true, true, true, false)
}

func writeDownloaderCompanion(t *testing.T, directory string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, "downloader-companion"), nil, 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertDownloaderStatusLog(t *testing.T, output string, enabled bool, present bool, working bool, hasReason bool) {
	t.Helper()
	expected := fmt.Sprintf("downloader status enabled=%t present=%t working=%t", enabled, present, working)
	if !strings.Contains(output, expected) || strings.Contains(output, "reason=") != hasReason {
		t.Fatalf("unexpected downloader status log %q", output)
	}
}

func stringPointer(value string) *string { return &value }
