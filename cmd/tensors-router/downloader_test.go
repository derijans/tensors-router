package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"tensors-router/internal/config"
	"tensors-router/internal/downloader"
)

func TestDownloaderBinaryPathUsesConfiguredLocation(t *testing.T) {
	routerConfigPath := filepath.Join("configs", "router.yaml")

	path, err := downloaderBinaryPath(routerConfigPath, filepath.Join("tools", "tensor-router-downloader"))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.Abs(filepath.Join("configs", "tools", "tensor-router-downloader"))
	if err != nil {
		t.Fatal(err)
	}
	if path != expected {
		t.Fatalf("relative binary path %q was not resolved against the router configuration to %q", path, expected)
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
			manager, capability := optionalDownloader(filepath.Join(directory, "router.yaml"), config.DownloaderConfig{Enabled: true, BinaryLocation: downloaderCompanionTestName()}, log.New(io.Discard, "", 0))
			if manager != nil || !capability.Enabled || !capability.Present || capability.Working || !strings.Contains(capability.Reason, test.reason) {
				t.Fatalf("unexpected configuration failure status %#v", capability)
			}
		})
	}
}

func TestOptionalDownloaderStartsWithDefaultsWhenConfigurationIsMissing(t *testing.T) {
	directory := t.TempDir()
	writeDownloaderCompanion(t, directory)
	var output bytes.Buffer
	manager, capability := optionalDownloader(filepath.Join(directory, "router.yaml"), config.DownloaderConfig{Enabled: true, BinaryLocation: downloaderCompanionTestName()}, log.New(&output, "", 0))
	if manager == nil || !capability.Working {
		t.Fatalf("a missing downloader.yaml disabled the downloader: %#v", capability)
	}
	defer manager.Close()
	expectedRoot, err := filepath.EvalSymlinks(filepath.Join(directory, "models"))
	if err != nil {
		t.Fatal(err)
	}
	if capability.StorageRoot != expectedRoot || !strings.Contains(output.String(), "using defaults") {
		t.Fatalf("unexpected default storage %q or missing warning in %q", capability.StorageRoot, output.String())
	}
}

func TestModelFileRootsIncludeWorkingDownloaderStorage(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "models")
	working := downloader.Capability{Working: true, StorageRoot: storage}
	if roots := modelFileRoots([]string{"/srv/models"}, working); len(roots) != 2 || roots[1] != storage {
		t.Fatalf("downloader storage was not offered to cook and inventory: %v", roots)
	}
	if roots := modelFileRoots([]string{filepath.Dir(storage)}, working); len(roots) != 1 {
		t.Fatalf("storage already covered by a configured root was added twice: %v", roots)
	}
	if roots := modelFileRoots(nil, downloader.Capability{StorageRoot: storage}); len(roots) != 0 {
		t.Fatalf("storage of a non-working downloader was added: %v", roots)
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
			manager, capability := optionalDownloader(filepath.Join(directory, "router.yaml"), config.DownloaderConfig{Enabled: true, BinaryLocation: downloaderCompanionTestName()}, log.New(io.Discard, "", 0))
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
	manager, capability := optionalDownloader(filepath.Join(directory, "router.yaml"), config.DownloaderConfig{Enabled: true, BinaryLocation: downloaderCompanionTestName()}, log.New(&output, "", 0))
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
	command := exec.Command("go", "build", "-o", filepath.Join(directory, downloaderCompanionTestName()), "../tensor-router-downloader")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build downloader companion: %v\n%s", err, output)
	}
}

func downloaderCompanionTestName() string {
	if runtime.GOOS == "windows" {
		return "downloader-companion.exe"
	}
	return "downloader-companion"
}

func assertDownloaderStatusLog(t *testing.T, output string, enabled bool, present bool, working bool, hasReason bool) {
	t.Helper()
	expected := fmt.Sprintf("downloader status enabled=%t present=%t working=%t", enabled, present, working)
	if !strings.Contains(output, expected) || strings.Contains(output, "reason=") != hasReason {
		t.Fatalf("unexpected downloader status log %q", output)
	}
}

func stringPointer(value string) *string { return &value }
