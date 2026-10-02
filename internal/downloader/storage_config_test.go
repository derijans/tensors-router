package downloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigWithoutFileUsesDefaultsNextToIt(t *testing.T) {
	directory := t.TempDir()
	config, warnings, err := LoadConfig(filepath.Join(directory, "downloader.yaml"))
	if err != nil {
		t.Fatalf("a missing downloader.yaml disabled the downloader: %v", err)
	}
	if config.Storage.Root != filepath.Join(directory, "models") || len(warnings) == 0 {
		t.Fatalf("unexpected defaults %#v warnings=%v", config.Storage, warnings)
	}
}

func TestManagerCreatesMissingStorageParents(t *testing.T) {
	directory := t.TempDir()
	config := DefaultConfig(filepath.Join(directory, "downloader.yaml"))
	config.Logging.Mode = "off"
	config.Storage.Root = filepath.Join(directory, "not", "yet", "created", "models")
	manager, err := NewManager(config, "")
	if err != nil {
		t.Fatalf("storage root with missing parents was rejected: %v", err)
	}
	defer manager.Close()
	if info, err := os.Stat(config.Storage.Root); err != nil || !info.IsDir() {
		t.Fatalf("storage root was not created: %v", err)
	}
}

func TestManagerAcceptsLinkedStorageRoot(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "disk", "models")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "models-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are unavailable here: %v", err)
	}
	config := DefaultConfig(filepath.Join(directory, "downloader.yaml"))
	config.Logging.Mode = "off"
	config.Storage.Root = link
	manager, err := NewManager(config, "")
	if err != nil {
		t.Fatalf("a storage root reached through a link was rejected: %v", err)
	}
	defer manager.Close()
	if resolved, _ := filepath.EvalSymlinks(target); manager.config.Storage.Root != resolved {
		t.Fatalf("storage root %q was not resolved to %q", manager.config.Storage.Root, resolved)
	}
}

func TestHubEndpointComesFromEnvironmentAndRejectsPlainRemoteHTTP(t *testing.T) {
	t.Setenv("HF_ENDPOINT", "https://hf-mirror.example/")
	if endpoint, err := resolveHubEndpoint(""); err != nil || endpoint != "https://hf-mirror.example" {
		t.Fatalf("HF_ENDPOINT was not honoured: %q %v", endpoint, err)
	}
	if endpoint, err := resolveHubEndpoint("http://127.0.0.1:8080"); err != nil || endpoint != "http://127.0.0.1:8080" {
		t.Fatalf("loopback mirror was rejected: %q %v", endpoint, err)
	}
	if _, err := resolveHubEndpoint("http://mirror.example"); err == nil {
		t.Fatal("a plain-http remote endpoint would send tokens in clear text")
	}
}

func TestHubTokenFallsBackToEnvironment(t *testing.T) {
	t.Setenv("HF_TOKEN", "hf_environmenttoken")
	config, _, err := LoadConfig(filepath.Join(t.TempDir(), "downloader.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if config.HuggingFace.Token != "hf_environmenttoken" {
		t.Fatalf("HF_TOKEN was not used when the config has no token")
	}
}

func TestRedactionKeepsOrdinaryWordsAndHidesCredentials(t *testing.T) {
	message := redactSensitive(`download "tokenizer.json" failed: provide an authorized token; Authorization: Bearer abc.def hf_abcdefghijklmnop token=secret-value`)
	for _, kept := range []string{`"tokenizer.json"`, "authorized token;"} {
		if !strings.Contains(message, kept) {
			t.Errorf("redaction destroyed %q: %s", kept, message)
		}
	}
	for _, hidden := range []string{"abc.def", "hf_abcdefghijklmnop", "secret-value"} {
		if strings.Contains(message, hidden) {
			t.Errorf("redaction leaked %q: %s", hidden, message)
		}
	}
}
