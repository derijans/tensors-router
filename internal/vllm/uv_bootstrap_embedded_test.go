//go:build vllm_embedded_uv

package vllm

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTaggedBuildStagesEmbeddedUVExactly(t *testing.T) {
	if !embeddedUVAvailable() {
		t.Fatal("tagged build has empty uv bootstrap")
	}
	if _, err := EmbeddedUVBootstrap(); err != nil {
		t.Fatal(err)
	}
	binaryName := "uv"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	path := filepath.Join(t.TempDir(), binaryName)
	staged, err := stageEmbeddedUV(path)
	if err != nil || !staged {
		t.Fatalf("embedded bootstrap was not staged staged=%t error=%v", staged, err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, embeddedUV) {
		t.Fatal("staged uv differs from embedded bootstrap")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o500 != 0o500 {
		t.Fatalf("embedded uv is not executable: %o", info.Mode().Perm())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil || !strings.HasPrefix(string(output), "uv ") {
		t.Fatalf("embedded uv cannot execute on current platform: %v %s", err, output)
	}
}
