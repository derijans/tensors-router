package downloader

import (
	"os"
	"path/filepath"
	"strconv"
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

func TestSetConfigValueRejectsNativeIntOverflow(t *testing.T) {
	maximum := "9223372036854775807"
	overflow := "9223372036854775808"
	underflow := "-9223372036854775809"
	if strconv.IntSize == 32 {
		maximum = "2147483647"
		overflow = "2147483648"
		underflow = "-2147483649"
	}

	tests := []struct {
		section string
		key     string
	}{
		{section: "downloads", key: "concurrent_jobs"},
		{section: "downloads", key: "concurrent_files"},
		{section: "downloads", key: "retry_limit"},
		{section: "scanning", key: "hash_workers"},
		{section: "hardware", key: "default_context"},
		{section: "hardware", key: "safety_margin_percent"},
	}

	for _, testCase := range tests {
		t.Run(testCase.section+"_"+testCase.key, func(t *testing.T) {
			config := DefaultConfig(filepath.Join(t.TempDir(), "downloader.yaml"))
			if err := setConfigValue(&config, testCase.section, testCase.key, maximum); err != nil {
				t.Fatalf("accepted native integer value returned %v", err)
			}
			if err := setConfigValue(&config, testCase.section, testCase.key, overflow); err == nil {
				t.Fatal("native integer overflow was accepted")
			}
			if err := setConfigValue(&config, testCase.section, testCase.key, underflow); err == nil {
				t.Fatal("native integer underflow was accepted")
			}
		})
	}
}
