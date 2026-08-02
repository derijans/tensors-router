package downloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagerWritesConfiguredDownloaderLog(t *testing.T) {
	directory := t.TempDir()
	config := DefaultConfig(filepath.Join(directory, "downloader.yaml"))
	config.Storage.FreeSpaceReserveGB = 0
	manager, err := NewManager(config, "hf")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(directory, "data", "downloader.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "downloader initialized") {
		t.Fatalf("unexpected downloader log %q", string(content))
	}
}

func TestManagerCapabilityReportsCapacityInspectionFailure(t *testing.T) {
	directory := t.TempDir()
	config := DefaultConfig(filepath.Join(directory, "downloader.yaml"))
	config.Logging.Mode = "off"
	manager, err := NewManager(config, "hf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if err := os.Remove(config.Storage.Root); err != nil {
		t.Fatal(err)
	}
	capability := manager.Capability()
	if !strings.Contains(capability.Reason, "inspect downloader storage capacity") || capability.Error != capability.Reason {
		t.Fatalf("unexpected capacity failure %#v", capability)
	}
}

func TestMergeRuntimeCapabilityPreservesStartupState(t *testing.T) {
	startup := Capability{Enabled: true, Present: true, Working: true}
	merged := MergeRuntimeCapability(startup, Capability{Available: true, Configured: true, StorageRoot: "models", FreeBytes: 10})
	if !merged.Enabled || !merged.Present || !merged.Working || !merged.Available || !merged.Configured || merged.StorageRoot != "models" || merged.FreeBytes != 10 {
		t.Fatalf("startup status was not preserved %#v", merged)
	}

	failed := MergeRuntimeCapability(startup, Capability{Available: true, Configured: true, Error: "capacity failed"})
	if failed.Working || failed.Reason != "capacity failed" {
		t.Fatalf("runtime failure did not update working status %#v", failed)
	}
}

func TestPromotedAndRescannedArtifactsNotifyHandler(t *testing.T) {
	root := t.TempDir()
	manager, err := NewManager(Config{
		Storage:   StorageConfig{Root: root, StateDir: t.TempDir(), DatabasePath: filepath.Join(t.TempDir(), "downloads.db")},
		Downloads: DownloadsConfig{ConcurrentJobs: 1},
	}, "hf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	staged := filepath.Join(t.TempDir(), "model.gguf")
	content := []byte("downloaded model")
	if err := os.WriteFile(staged, content, 0o600); err != nil {
		t.Fatal(err)
	}
	hash, _, err := SHA256File(staged)
	if err != nil {
		t.Fatal(err)
	}
	var notified ArtifactRecord
	manager.SetArtifactHandler(func(record ArtifactRecord) error {
		notified = record
		return nil
	})
	job := DownloadJob{ID: "job", Repository: "owner/repository", Commit: "0123456789abcdef0123456789abcdef01234567"}
	file := JobFile{Path: "model.gguf"}
	if err := manager.promote(job, file, staged, hash); err != nil {
		t.Fatal(err)
	}
	if notified.SHA256 != hash || notified.Repository != job.Repository || notified.RepositoryPath != file.Path {
		t.Fatalf("promoted artifact was not notified: %#v", notified)
	}
	notified = ArtifactRecord{}
	if _, err := manager.Rescan(); err != nil {
		t.Fatal(err)
	}
	if notified.SHA256 != hash || notified.Path == "" {
		t.Fatalf("rescanned artifact was not notified: %#v", notified)
	}
}
