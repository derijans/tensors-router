package vllm

import (
	"strings"
	"testing"
)

func TestTUFManifestSourceRejectsNormalizedTraversal(t *testing.T) {
	source := TUFManifestSource{
		RepositoryURL: "https://updates.example.test/metadata",
		TrustedRoot:   []byte("root"),
		TargetPath:    "../profiles/linux-amd64.json",
		CacheDir:      t.TempDir(),
	}
	if _, _, _, err := source.validate(); err == nil || !strings.Contains(err.Error(), "target path") {
		t.Fatalf("TUF traversal target was accepted: %v", err)
	}
	source.TargetPath = "profiles/../linux-amd64.json"
	if _, _, _, err := source.validate(); err == nil || !strings.Contains(err.Error(), "target path") {
		t.Fatalf("normalized TUF target was accepted: %v", err)
	}
}
