package catalog

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
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

func TestRuntimeConfigParsesRouterUnloadPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "strict.kcpps")
	writeCatalogFile(t, dir, "strict.kcpps", `{"router_unload_policy":"image","model_param":"text.gguf"}`)

	metadata, err := LoadRuntimeConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.RouterUnloadPolicy != "image" {
		t.Fatalf("unexpected unload policy %q", metadata.RouterUnloadPolicy)
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
		"talkermodel":"C:/models/talker.gguf",
		"code2wavmodel":"C:/models/code2wav.gguf",
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
	if model.Capabilities.Voice == nil || model.Capabilities.Voice.TTSModel != "C:/models/tts.gguf" || model.Capabilities.Voice.TalkerModel != "C:/models/talker.gguf" || model.Capabilities.Voice.Code2WAVModel != "C:/models/code2wav.gguf" || !model.Capabilities.Voice.GPU {
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

func TestChatTemplateProfileKeepsVirtualVariantsDistinctAndPhysicallyCompatible(t *testing.T) {
	think := []byte(`{"model_param":"C:/models/model.gguf","jinja_kwargs":"{\"enable_thinking\":true,\"mode\":\"chat\"}"}`)
	noThink := []byte(`{"model_param":"C:/models/model.gguf","jinja_kwargs":{"enable_thinking":false,"mode":"chat"}}`)
	differentKeys := []byte(`{"model_param":"C:/models/model.gguf","jinja_kwargs":{"enable_thinking":false,"reasoning_format":"deepseek"}}`)
	differentRuntime := []byte(`{"model_param":"C:/models/model.gguf","contextsize":8192,"jinja_kwargs":{"enable_thinking":false,"mode":"chat"}}`)

	thinkProfile := ChatTemplateProfileForConfig(think)
	noThinkProfile := ChatTemplateProfileForConfig(noThink)
	differentKeysProfile := ChatTemplateProfileForConfig(differentKeys)
	differentRuntimeProfile := ChatTemplateProfileForConfig(differentRuntime)

	if !thinkProfile.Valid() || !noThinkProfile.Valid() {
		t.Fatalf("expected valid profiles think=%#v noThink=%#v", thinkProfile, noThinkProfile)
	}
	if ConfigHash(think) == ConfigHash(noThink) {
		t.Fatal("virtual variants must retain separate config hashes")
	}
	if !thinkProfile.SharesPhysicalRuntimeWith(noThinkProfile) {
		t.Fatalf("matching Jinja key sets should share a physical runtime: %q %q", thinkProfile.PhysicalLoadFingerprint(), noThinkProfile.PhysicalLoadFingerprint())
	}
	if thinkProfile.SharesPhysicalRuntimeWith(differentKeysProfile) {
		t.Fatal("different Jinja key sets must not share a physical runtime")
	}
	if thinkProfile.SharesPhysicalRuntimeWith(differentRuntimeProfile) {
		t.Fatal("different non-Jinja config values must not share a physical runtime")
	}
	if thinkProfile.Precedence() != JinjaKwargsPrecedenceConfig {
		t.Fatalf("unexpected default precedence %q", thinkProfile.Precedence())
	}
}

func TestMalformedJinjaKwargsRemainCatalogedButDoNotShareRuntime(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "manual.kcpps", `{"model_param":"model.gguf","jinja_kwargs":"not-json"}`)
	models, err := New(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "manual" {
		t.Fatalf("manual config disappeared: %#v", models)
	}
	if models[0].ChatTemplate.Valid() {
		t.Fatal("malformed Jinja kwargs must not produce a reusable profile")
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
	if err := models.Flush(); err != nil {
		t.Fatal(err)
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
	if err := models.Refresh(); err != nil {
		t.Fatal(err)
	}
	if err := models.Flush(); err != nil {
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

func TestCatalogRefreshPublishesCompleteSnapshots(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "a.kcpps", `{}`)
	catalog := New(dir)
	writeCatalogFile(t, dir, "b.kcpps", `{}`)

	models, err := catalog.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("catalog changed without refresh %#v", models)
	}
	models[0].ID = "mutated"
	if err := catalog.Refresh(); err != nil {
		t.Fatal(err)
	}
	models, err = catalog.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "a" || models[1].ID != "b" {
		t.Fatalf("refresh did not publish complete cloned snapshot %#v", models)
	}
}

func TestCatalogKeepsLastSnapshotWhenRefreshFails(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFile(t, dir, "a.kcpps", `{}`)
	catalog := New(dir)
	catalog.readDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("scan failed") }
	if err := catalog.Refresh(); err == nil {
		t.Fatal("expected refresh failure")
	}
	models, err := catalog.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].ID != "a" {
		t.Fatalf("failed refresh replaced snapshot %#v", models)
	}
}

func TestHashStoreDebouncesDirtyWrites(t *testing.T) {
	store, err := NewHashStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.flushDelay = 20 * time.Millisecond
	var writes atomic.Int32
	written := make(chan struct{}, 2)
	store.persist = func(hashCache) error {
		writes.Add(1)
		written <- struct{}{}
		return nil
	}
	first := filepath.Join(t.TempDir(), "first.gguf")
	second := filepath.Join(t.TempDir(), "second.gguf")
	if err := os.WriteFile(first, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.StartScan()
	if _, ok := store.HashFile(first); !ok {
		t.Fatal("first hash failed")
	}
	if _, ok := store.HashFile(second); !ok {
		t.Fatal("second hash failed")
	}
	store.FinishScan()
	select {
	case <-written:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("debounced flush did not run")
	}
	time.Sleep(60 * time.Millisecond)
	if writes.Load() != 1 {
		t.Fatalf("expected one debounced write, got %d", writes.Load())
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
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
