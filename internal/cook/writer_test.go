package cook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/catalog"
)

func TestWriterReusesSingleExistingConfig(t *testing.T) {
	dir := packageTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "combo.kcpps"), []byte(`{
		"model_param":"text.gguf",
		"sdmodel":"dream.safetensors",
		"embeddingsmodel":"embed.gguf"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := Writer{ConfigDir: dir, Catalog: catalog.New(dir), NodeID: "node-a"}
	result, err := writer.Preview(NodeConfigRequest{
		ID: "mixed",
		Components: []Component{
			{Kind: KindText, Source: SourceConfig, ModelID: "combo"},
			{Kind: KindImage, Source: SourceConfig, ImageID: "combo-dream"},
			{Kind: KindEmbeddings, Source: SourceConfig, ModelID: "combo"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Reused || result.ModelID != "combo" || result.ImageID != "combo-dream" {
		t.Fatalf("unexpected reuse result %#v", result)
	}
}

func TestWriterComposesConfigAndPreservesUnknownKeys(t *testing.T) {
	dir := packageTempDir(t)
	root := packageTempDir(t)
	textPath := filepath.Join(root, "text.gguf")
	if err := os.WriteFile(textPath, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "image.kcpps"), []byte(`{
		"custom":"keep",
		"sdmodel":"C:/models/dream.safetensors",
		"sdvae":"C:/models/vae.safetensors"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := Writer{ConfigDir: dir, FileRoots: []string{root}, Catalog: catalog.New(dir), NodeID: "node-a"}
	result, err := writer.Apply(NodeConfigRequest{
		ID: "Cooked Model",
		Components: []Component{
			{Kind: KindText, Source: SourceFile, FilePath: textPath},
			{Kind: KindImage, Source: SourceConfig, ImageID: "image-dream"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ModelID != "cooked-model" || result.ImageID != "cooked-model-dream" {
		t.Fatalf("unexpected result %#v", result)
	}

	content, err := os.ReadFile(filepath.Join(dir, "cooked-model.kcpps"))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(content, &body); err != nil {
		t.Fatal(err)
	}
	if body["custom"] != "keep" || body["model_param"] != textPath || body["sdvae"] != "C:/models/vae.safetensors" {
		t.Fatalf("unexpected body %#v", body)
	}
}

func TestWriterRejectsConflictWithoutOverwrite(t *testing.T) {
	dir := packageTempDir(t)
	root := packageTempDir(t)
	textPath := filepath.Join(root, "text.gguf")
	if err := os.WriteFile(textPath, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mixed.kcpps"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := Writer{ConfigDir: dir, FileRoots: []string{root}, Catalog: catalog.New(dir), NodeID: "node-a"}
	_, err := writer.Apply(NodeConfigRequest{
		ID: "mixed",
		Components: []Component{
			{Kind: KindText, Source: SourceFile, FilePath: textPath},
		},
	})
	if err == nil {
		t.Fatalf("expected conflict error")
	}
}

func TestWriterAppliesOptionOverrides(t *testing.T) {
	dir := packageTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "image.kcpps"), []byte(`{
		"nomodel":true,
		"sdmodel":"C:/models/dream.safetensors",
		"sdthreads":4
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := Writer{ConfigDir: dir, Catalog: catalog.New(dir), NodeID: "node-a"}
	result, err := writer.Apply(NodeConfigRequest{
		ID: "override",
		Components: []Component{
			{Kind: KindImage, Source: SourceConfig, ImageID: "image-dream"},
		},
		Options: Options{
			"sdmodel":   json.RawMessage(`"D:/models/neon.safetensors"`),
			"sdthreads": json.RawMessage(`9`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ImageID != "override-neon" {
		t.Fatalf("unexpected image id %#v", result)
	}
	content, err := os.ReadFile(filepath.Join(dir, "override.kcpps"))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(content, &body); err != nil {
		t.Fatal(err)
	}
	if body["sdmodel"] != "D:/models/neon.safetensors" || body["sdthreads"].(float64) != 9 {
		t.Fatalf("override options were not applied: %#v", body)
	}
}

func TestWriterPreservesAndOverridesBackendMode(t *testing.T) {
	dir := packageTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "native.kcpps"), []byte(`{
		"backend_mode":"llama_sdcpp",
		"model_param":"text.gguf"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writer := Writer{ConfigDir: dir, Catalog: catalog.New(dir), NodeID: "node-a"}
	components := []Component{{Kind: KindText, Source: SourceConfig, ModelID: "native"}}

	body, _, err := writer.composedConfig(components, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(body["backend_mode"]) != `"llama_sdcpp"` {
		t.Fatalf("source backend_mode was not preserved: %s", body["backend_mode"])
	}

	body, _, err = writer.composedConfig(components, Options{"backend_mode": rawJSON(t, "kobold")})
	if err != nil {
		t.Fatal(err)
	}
	if string(body["backend_mode"]) != `"kobold"` {
		t.Fatalf("backend_mode override was not applied: %s", body["backend_mode"])
	}
}

func TestWriterComposesVoiceMusicRawFilesWithOptionKeys(t *testing.T) {
	dir := packageTempDir(t)
	root := packageTempDir(t)
	voicePath := filepath.Join(root, "voice.gguf")
	musicPath := filepath.Join(root, "musicvae.gguf")
	if err := os.WriteFile(voicePath, []byte("voice"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(musicPath, []byte("music"), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := Writer{ConfigDir: dir, FileRoots: []string{root}, Catalog: catalog.New(dir), NodeID: "node-a"}
	_, err := writer.Apply(NodeConfigRequest{
		ID: "audio",
		Components: []Component{
			{Kind: KindVoice, Source: SourceFile, FilePath: voicePath, OptionKey: "ttsmodel"},
			{Kind: KindMusic, Source: SourceFile, FilePath: musicPath, OptionKey: "musicvae"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := readConfigBody(t, filepath.Join(dir, "audio.kcpps"))
	if body["ttsmodel"] != voicePath || body["musicvae"] != musicPath || body["nomodel"] != true {
		t.Fatalf("unexpected audio body %#v", body)
	}
}

func TestWriterRejectsVoiceMusicRawFileWithoutOptionKey(t *testing.T) {
	dir := packageTempDir(t)
	root := packageTempDir(t)
	voicePath := filepath.Join(root, "voice.gguf")
	if err := os.WriteFile(voicePath, []byte("voice"), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := Writer{ConfigDir: dir, FileRoots: []string{root}, Catalog: catalog.New(dir), NodeID: "node-a"}
	_, err := writer.Preview(NodeConfigRequest{
		ID: "audio",
		Components: []Component{
			{Kind: KindVoice, Source: SourceFile, FilePath: voicePath},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `voice option key "" is invalid`) {
		t.Fatalf("expected option key validation error, got %v", err)
	}
}

func TestWriterCopiesOnlySelectedVoiceMusicAndSharedConfigKeys(t *testing.T) {
	dir := packageTempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "voice.kcpps"), []byte(`{
		"quiet":true,
		"custom_backend_key":"keep",
		"model_param":"text.gguf",
		"sdmodel":"image.safetensors",
		"whispermodel":"whisper.gguf"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "music.kcpps"), []byte(`{
		"sdmodel":"other-image.safetensors",
		"musicdiffusion":"music-diffusion.gguf"
	}`), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := Writer{ConfigDir: dir, Catalog: catalog.New(dir), NodeID: "node-a"}
	_, err := writer.Apply(NodeConfigRequest{
		ID: "audio-shared",
		Components: []Component{
			{Kind: KindVoice, Source: SourceConfig, ModelID: "voice"},
			{Kind: KindMusic, Source: SourceConfig, ModelID: "music"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := readConfigBody(t, filepath.Join(dir, "audio-shared.kcpps"))
	if body["quiet"] != true || body["custom_backend_key"] != "keep" || body["whispermodel"] != "whisper.gguf" || body["musicdiffusion"] != "music-diffusion.gguf" || body["nomodel"] != true {
		t.Fatalf("unexpected shared audio body %#v", body)
	}
	if _, ok := body["model_param"]; ok {
		t.Fatalf("text option leaked into audio body %#v", body)
	}
	if _, ok := body["sdmodel"]; ok {
		t.Fatalf("image option leaked into audio body %#v", body)
	}
}

func TestWriterRejectsRawFileTraversal(t *testing.T) {
	base := packageTempDir(t)
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(base, "configs")
	outsidePath := filepath.Join(base, "outside.gguf")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := Writer{ConfigDir: configDir, FileRoots: []string{root}, Catalog: catalog.New(configDir), NodeID: "node-a"}
	_, err := writer.Preview(NodeConfigRequest{
		ID: "made",
		Components: []Component{
			{Kind: KindText, Source: SourceFile, FilePath: filepath.Join(root, "..", "outside.gguf")},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "outside configured model roots") {
		t.Fatalf("expected root escape rejection, got %v", err)
	}
}

func TestWriterRejectsRawFileSymlinkEscape(t *testing.T) {
	base := packageTempDir(t)
	root := filepath.Join(base, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(base, "configs")
	outsidePath := filepath.Join(base, "outside.gguf")
	if err := os.WriteFile(outsidePath, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(root, "outside.gguf")
	if err := os.Symlink(outsidePath, linkPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	writer := Writer{ConfigDir: configDir, FileRoots: []string{root}, Catalog: catalog.New(configDir), NodeID: "node-a"}
	_, err := writer.Preview(NodeConfigRequest{
		ID: "made",
		Components: []Component{
			{Kind: KindText, Source: SourceFile, FilePath: linkPath},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "outside configured model roots") {
		t.Fatalf("expected symlink escape rejection, got %v", err)
	}
}

func TestWriterRejectsPathLikeConfigID(t *testing.T) {
	dir := packageTempDir(t)
	root := packageTempDir(t)
	textPath := filepath.Join(root, "text.gguf")
	if err := os.WriteFile(textPath, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}

	writer := Writer{ConfigDir: dir, FileRoots: []string{root}, Catalog: catalog.New(dir), NodeID: "node-a"}
	_, err := writer.Preview(NodeConfigRequest{
		ID: "../made",
		Components: []Component{
			{Kind: KindText, Source: SourceFile, FilePath: textPath},
		},
	})
	if err == nil {
		t.Fatalf("expected path-like id rejection")
	}
}

func readConfigBody(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(content, &body); err != nil {
		t.Fatal(err)
	}
	return body
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
