//go:build !vllm_embedded_uv

package vllm

import (
	"path/filepath"
	"testing"
)

func TestDefaultBuildRequiresAuthorizedUVFallback(t *testing.T) {
	if embeddedUVAvailable() {
		t.Fatal("default build unexpectedly contains uv")
	}
	if _, err := EmbeddedUVBootstrap(); err == nil {
		t.Fatal("default build reported embedded uv")
	}
	staged, err := stageEmbeddedUV(filepath.Join(t.TempDir(), "uv"))
	if err != nil || staged {
		t.Fatalf("unexpected default bootstrap stage staged=%t error=%v", staged, err)
	}
}
