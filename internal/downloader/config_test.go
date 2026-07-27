package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigResolvesStatePathsAndRejectsExternalDatabase(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "downloader.yaml")
	content := `
storage:
  root: "./models"
  state_dir: "./state"
  database_path: "./state/downloads.sqlite"
  free_space_reserve_gb: 7

huggingface:
  token: ""

downloads:
  concurrent_jobs: 3
  concurrent_files: 4
  retry_limit: 5
  timeout: "30s"

scanning:
  hash_workers: 1
  write_hash_sidecars: true

hardware:
  default_context: 8192
  vram_reserve_mb: 1024
  safety_margin_percent: 15

logging:
  mode: "normal"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	config, _, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Storage.Root != filepath.Join(directory, "models") || config.Storage.DatabasePath != filepath.Join(directory, "state", "downloads.sqlite") || config.Storage.FreeSpaceReserveGB != 7 {
		t.Fatalf("unexpected config %#v", config.Storage)
	}
	if err := os.WriteFile(path, []byte("storage:\n  database_path: ../outside.sqlite\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadConfig(path); err == nil {
		t.Fatal("expected database path validation error")
	}
}

func TestRepositoryDestinationRejectsTraversal(t *testing.T) {
	directory := t.TempDir()
	for _, repository := range []string{"owner", "../owner/model", "owner/../model", "owner/model/extra"} {
		if _, err := RepositoryDirectory(directory, repository); err == nil {
			t.Fatalf("expected repository %q to be rejected", repository)
		}
	}
	for _, file := range []string{"../model.gguf", "/model.gguf", `folder\\model.gguf`, ""} {
		if _, err := DestinationPath(directory, "owner/model", file); err == nil {
			t.Fatalf("expected file %q to be rejected", file)
		}
	}
	destination, err := DestinationPath(directory, "owner/model", "weights/model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if !pathWithin(destination, directory) {
		t.Fatalf("destination escaped root %q", destination)
	}
}
