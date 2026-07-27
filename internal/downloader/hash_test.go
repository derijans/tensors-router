package downloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSHA256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestTrustedHashSidecarImportsSingleStem(t *testing.T) {
	directory := t.TempDir()
	model := filepath.Join(directory, "model.gguf")
	if err := os.WriteFile(model, []byte("model"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "model.hash"), []byte(testSHA256+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hash, trusted, err := ReadTrustedHashSidecar(model)
	if err != nil {
		t.Fatal(err)
	}
	if !trusted || hash != testSHA256 {
		t.Fatalf("unexpected sidecar hash=%q trusted=%t", hash, trusted)
	}
}

func TestAmbiguousHashSidecarIsNotTrustedOrChanged(t *testing.T) {
	directory := t.TempDir()
	gguf := filepath.Join(directory, "model.gguf")
	safetensors := filepath.Join(directory, "model.safetensors")
	sidecar := filepath.Join(directory, "model.hash")
	for _, path := range []string{gguf, safetensors} {
		if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(sidecar, []byte(testSHA256+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, trusted, err := ReadTrustedHashSidecar(gguf); err != nil || trusted {
		t.Fatalf("expected ambiguous sidecar to be ignored trusted=%t err=%v", trusted, err)
	}
	if err := WriteHashSidecar(gguf, "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != testSHA256+"\n" {
		t.Fatalf("ambiguous sidecar changed to %q", content)
	}
}

func TestIsolatedEnvironmentDoesNotUseGlobalToken(t *testing.T) {
	t.Setenv("HF_TOKEN", "global-token")
	environment := isolatedHFEnvironment(t.TempDir(), "operation-token")
	values := map[string]string{}
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}
	if values["HF_TOKEN"] != "operation-token" || values["HF_HUB_DISABLE_IMPLICIT_TOKEN"] != "1" || values["HF_HUB_DISABLE_TELEMETRY"] != "1" || values["HF_HUB_DISABLE_UPDATE_CHECK"] != "1" {
		t.Fatalf("unexpected isolated environment %#v", values)
	}
}
