package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestListReturnsPublicIDsWithoutKcppsExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.kcpps"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.kcpps"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder.kcpps"), 0o755); err != nil {
		t.Fatal(err)
	}

	models, err := New(dir).List()
	if err != nil {
		t.Fatal(err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "a" || models[1].ID != "b" {
		t.Fatalf("unexpected model order %#v", models)
	}
	if models[0].Filename != "a.kcpps" || models[1].Filename != "b.kcpps" {
		t.Fatalf("unexpected filenames %#v", models)
	}
}

func TestResolveRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.kcpps"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok, err := New(dir).Resolve("../a")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("path traversal should not match")
	}
}

func TestResolveMapsPublicIDToConfigFilename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.kcpps"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	model, ok, err := New(dir).Resolve("a")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected model")
	}
	if model.Filename != "a.kcpps" {
		t.Fatalf("unexpected filename %q", model.Filename)
	}
}

func TestCatalogParsesOptionalBackendMode(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "default.kcpps", `{}`)
	writeCatalogFile(t, dir, "native.kcpps", `{"backend_mode":"llama_sdcpp","model_param":"text.gguf"}`)

	models, err := New(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if models[0].BackendMode != "" {
		t.Fatalf("missing backend_mode should remain empty, got %q", models[0].BackendMode)
	}
	if models[1].BackendMode != "llama_sdcpp" {
		t.Fatalf("backend_mode was not parsed: %#v", models[1])
	}
}

func TestMissingDirectoryReturnsEmptyCatalog(t *testing.T) {
	models, err := New(filepath.Join(t.TempDir(), "missing")).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 0 {
		t.Fatalf("expected empty model list")
	}
}

func TestImageModelsUseVirtualIDsAndActiveCombinedConfig(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "llm.kcpps", `{}`)
	writeCatalogFile(t, dir, "image.kcpps", `{"nomodel":true,"sdmodel":"C:\\models\\dream.safetensors"}`)
	writeCatalogFile(t, dir, "combo.kcpps", `{"model_param":"C:\\models\\llm.gguf","sdmodel":"/models/vision.ckpt"}`)

	models, err := New(dir).ListLLM()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "combo" || models[1].ID != "llm" {
		t.Fatalf("unexpected llm models %#v", models)
	}

	images, err := New(dir).ListImages("")
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 || images[0].ImageID != "image-dream" {
		t.Fatalf("unexpected inactive images %#v", images)
	}

	images, err = New(dir).ListImages("combo.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 2 || images[0].ImageID != "combo-vision" || images[1].ImageID != "image-dream" {
		t.Fatalf("unexpected active images %#v", images)
	}

	_, ok, err := New(dir).ResolveImage("combo-vision", "")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("inactive combined image should not resolve")
	}

	model, ok, err := New(dir).ResolveImage("combo-vision", "combo.kcpps")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || model.Filename != "combo.kcpps" {
		t.Fatalf("unexpected active combined resolve %#v %v", model, ok)
	}
}

func writeCatalogFile(t *testing.T, dir string, filename string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCapabilitiesIncludeImageEmbeddingsMultimodalAndContext(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "rich.kcpps", `{
		"model_param":"C:/models/text.gguf",
		"contextsize":8192,
		"sdmodel":"C:/models/dream.safetensors",
		"sdupscaler":"C:/models/upscale.pth",
		"sdvae":"C:/models/vae.safetensors",
		"sdvaeauto":true,
		"sdt5xxl":"C:/models/t5.safetensors",
		"sdclip1":"C:/models/clip-l.safetensors",
		"sdclip2":"C:/models/clip-g.safetensors",
		"sdlora":["C:/models/style.safetensors"],
		"sdquant":2,
		"sdtiledvae":768,
		"sdflashattention":true,
		"sdoffloadcpu":true,
		"sdvaecpu":true,
		"sdclipgpu":true,
		"embeddingsmodel":"C:/models/embed.gguf",
		"embeddingsmaxctx":2048,
		"embeddingsgpu":true,
		"mmproj":"C:/models/mmproj.gguf",
		"visionmaxres":1024,
		"visionmintokens":32,
		"visionmaxtokens":512,
		"whispermodel":"C:/models/whisper.gguf",
		"ttsmodel":"C:/models/tts.gguf",
		"ttswavtokenizer":"C:/models/tokenizer.gguf",
		"ttsdir":"C:/voices",
		"ttsgpu":true,
		"musicllm":"C:/music/llm.gguf",
		"musicembeddings":"C:/music/embed.gguf",
		"musicdiffusion":"C:/music/diffusion.gguf",
		"musicvae":"C:/music/vae.gguf",
		"musiclowvram":true
	}`)

	models, err := New(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	model := models[0]
	if !model.HasLLM || !model.HasImage || !model.HasEmbeddings || !model.HasMultimodal || !model.HasVoice || !model.HasMusic {
		t.Fatalf("unexpected capability booleans %#v", model)
	}
	if model.Capabilities.Context != 8192 {
		t.Fatalf("unexpected context %d", model.Capabilities.Context)
	}
	if model.Capabilities.Image == nil || model.Capabilities.Image.Upscaler == "" || model.Capabilities.Image.VAE == "" {
		t.Fatalf("missing image details %#v", model.Capabilities.Image)
	}
	if model.Capabilities.Embeddings == nil || model.Capabilities.Embeddings.MaxCtx != 2048 || !model.Capabilities.Embeddings.GPU {
		t.Fatalf("missing embedding details %#v", model.Capabilities.Embeddings)
	}
	if model.Capabilities.Multimodal == nil || model.Capabilities.Multimodal.VisionMaxRes != 1024 {
		t.Fatalf("missing multimodal details %#v", model.Capabilities.Multimodal)
	}
	if model.Capabilities.Voice == nil || model.Capabilities.Voice.TTSModel != "C:/models/tts.gguf" || !model.Capabilities.Voice.GPU {
		t.Fatalf("missing voice details %#v", model.Capabilities.Voice)
	}
	if model.Capabilities.Music == nil || model.Capabilities.Music.Diffusion != "C:/music/diffusion.gguf" || !model.Capabilities.Music.LowVRAM {
		t.Fatalf("missing music details %#v", model.Capabilities.Music)
	}
}

func TestConfigHashIgnoresPathsButKeepsRuntimeValues(t *testing.T) {
	first := configHashJSON(t, map[string]any{
		"model_param": "C:/models/a.gguf",
		"admindir":    "C:/router/a",
		"contextsize": 4096,
		"quantkv":     "f16",
	})
	second := configHashJSON(t, map[string]any{
		"model_param": "/mnt/models/a.gguf",
		"admindir":    "/router/b",
		"contextsize": 4096,
		"quantkv":     "f16",
	})
	changedContext := configHashJSON(t, map[string]any{
		"model_param": "/mnt/models/a.gguf",
		"admindir":    "/router/b",
		"contextsize": 8192,
		"quantkv":     "f16",
	})

	if ConfigHash(first) != ConfigHash(second) {
		t.Fatalf("path-only changes should not change config hash")
	}
	if ConfigHash(first) == ConfigHash(changedContext) {
		t.Fatalf("runtime changes should change config hash")
	}
}

func TestHashStoreCachesFileHashesAndDropsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	storeDir := t.TempDir()
	modelPath := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("weights"), 0o644); err != nil {
		t.Fatal(err)
	}
	content := configHashJSON(t, map[string]any{"model_param": modelPath})
	writeCatalogFile(t, dir, "model.kcpps", string(content))

	models, err := NewWithStore(dir, storeDir)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := models.List()
	if err != nil {
		t.Fatal(err)
	}
	if listed[0].ModelHash == "" {
		t.Fatalf("expected model hash")
	}
	cachePath := filepath.Join(storeDir, "hash-cache.json")
	cache, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !cacheHasPath(t, cache, modelPath) {
		t.Fatalf("cache missing model path: %s", string(cache))
	}

	if err := os.Remove(modelPath); err != nil {
		t.Fatal(err)
	}
	if _, err := models.List(); err != nil {
		t.Fatal(err)
	}
	cache, err = os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if cacheHasPath(t, cache, modelPath) {
		t.Fatalf("cache kept missing model path: %s", string(cache))
	}
}

func configHashJSON(t *testing.T, value map[string]any) []byte {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func cacheHasPath(t *testing.T, content []byte, path string) bool {
	t.Helper()
	var cache hashCache
	if err := json.Unmarshal(content, &cache); err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	_, ok := cache.Files[absolute]
	return ok
}
