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

func TestScanInfersVoiceAndMusicRolesFromConfigReferences(t *testing.T) {
	root := t.TempDir()
	voicePath := filepath.Join(root, "tts.gguf")
	musicPath := filepath.Join(root, "music-diffusion.gguf")
	if err := os.WriteFile(voicePath, []byte("voice"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(musicPath, []byte("music"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Scan([]string{root}, []cluster.Model{{
		PublicID: "audio",
		Capabilities: catalog.Capabilities{
			Voice: &catalog.VoiceCapabilities{
				TTSModel: voicePath,
			},
			Music: &catalog.MusicCapabilities{
				Diffusion: musicPath,
			},
		},
	}}, "node-a")
	if err != nil {
		t.Fatal(err)
	}

	voice := findFileRecord(files, voicePath)
	if voice == nil || !hasRole(voice.Roles, RoleVoice) || len(voice.ReferencedBy) != 1 || voice.ReferencedBy[0] != "audio" {
		t.Fatalf("missing voice role/reference %#v", voice)
	}
	music := findFileRecord(files, musicPath)
	if music == nil || !hasRole(music.Roles, RoleMusic) || len(music.ReferencedBy) != 1 || music.ReferencedBy[0] != "audio" {
		t.Fatalf("missing music role/reference %#v", music)
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

func findFileRecord(files []FileRecord, path string) *FileRecord {
	clean := filepath.Clean(path)
	for index := range files {
		if files[index].Path == clean {
			return &files[index]
		}
	}
	return nil
}

func hasRole(roles []string, role string) bool {
	for _, value := range roles {
		if value == role {
			return true
		}
	}
	return false
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
