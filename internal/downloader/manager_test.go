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
