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
	destination := filepath.Join(root, "owner", "repository", "model.gguf")
	promoted, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(promoted) != string(content) {
		t.Fatalf("unexpected promoted content %q", promoted)
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

func TestStagingDirectoryUsesLocalStateStorage(t *testing.T) {
	storageRoot := t.TempDir()
	stateDir := t.TempDir()
	manager := &Manager{config: Config{Storage: StorageConfig{Root: storageRoot, StateDir: stateDir}}}
	staging, err := manager.stagingDirectory(DownloadJob{ID: "job", Repository: "owner/repository"})
	if err != nil {
		t.Fatal(err)
	}
	if !pathWithin(staging, stateDir) || pathWithin(staging, storageRoot) {
		t.Fatalf("staging path %q was not isolated under state storage %q", staging, stateDir)
	}
}

func TestCopyPromotionFilePreservesVerifiedContent(t *testing.T) {
	content := []byte("cross-filesystem download")
	stagedPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(stagedPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	hash, _, err := SHA256File(stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "model.gguf")
	temporaryPath, err := copyPromotionFile(stagedPath, destination, hash)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(temporaryPath) })
	copied, err := os.ReadFile(temporaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(copied) != string(content) {
		t.Fatalf("unexpected copied content %q", copied)
	}
	if _, err := os.Stat(stagedPath); err != nil {
		t.Fatalf("cross-filesystem copy removed staging content: %v", err)
	}
}
