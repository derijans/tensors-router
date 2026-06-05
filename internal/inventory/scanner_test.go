package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"tensors-router/internal/catalog"
	"tensors-router/internal/cluster"
)

func TestScanClassifiesReferencedFiles(t *testing.T) {
	root := packageTempDir(t)
	imagePath := filepath.Join(root, "dream.safetensors")
	embedPath := filepath.Join(root, "embed.gguf")
	if err := os.WriteFile(imagePath, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(embedPath, []byte("embed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.txt"), []byte("no"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Scan([]string{root}, []cluster.Model{{
		PublicID:      "combo",
		LocalID:       "combo",
		HasImage:      true,
		HasEmbeddings: true,
		Capabilities: catalog.Capabilities{
			Image:      &catalog.ImageCapabilities{Model: imagePath},
			Embeddings: &catalog.EmbeddingCapability{Model: embedPath},
		},
	}}, "node-a")
	if err != nil {
		t.Fatal(err)
	}

	roles := map[string]string{}
	for _, file := range files {
		roles[file.Basename] = file.Role
		if file.NodeID != "node-a" {
			t.Fatalf("unexpected node id %q", file.NodeID)
		}
	}
	if roles["dream.safetensors"] != RoleImage {
		t.Fatalf("expected image role, got %#v", roles)
	}
	if roles["embed.gguf"] != RoleEmbeddings {
		t.Fatalf("expected embeddings role, got %#v", roles)
	}
	if _, ok := roles["ignored.txt"]; ok {
		t.Fatalf("txt file should not be scanned: %#v", roles)
	}
}

func TestScanSkipsSymlinkEscapes(t *testing.T) {
	root := packageTempDir(t)
	outside := packageTempDir(t)
	outsideFile := filepath.Join(outside, "outside.gguf")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "linked.gguf")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	files, err := Scan([]string{root}, nil, "node-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.Basename == "linked.gguf" {
			t.Fatalf("symlink escape was scanned: %#v", files)
		}
	}
}

func packageTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "tmp-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	absolute, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}
