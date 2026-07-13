package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type Catalog struct {
	refreshMu sync.Mutex
	dir       string
	hashStore *HashStore
	snapshot  atomic.Pointer[catalogSnapshot]
	readDir   func(string) ([]os.DirEntry, error)
}

type catalogSnapshot struct {
	models  []Model
	byID    map[string]Model
	loadErr error
}

const AllImageConfigs = "*"

type Model struct {
	ID             string
	Filename       string
	Path           string
	Created        int64
	HasLLM         bool
	HasImage       bool
	HasEmbeddings  bool
	HasMultimodal  bool
	HasVoice       bool
	HasMusic       bool
	ImageID        string
	ImageModelName string
	ImageModelPath string
	ModelHash      string
	ConfigHash     string
	BackendMode    string
	Capabilities   Capabilities
	Options        map[string]json.RawMessage
	ChatTemplate   ChatTemplateProfile `json:"-"`
}

func New(dir string) *Catalog {
	catalog := &Catalog{dir: dir, readDir: os.ReadDir}
	if err := catalog.Refresh(); err != nil {
		catalog.snapshot.Store(newCatalogSnapshot(nil, err))
	}
	return catalog
}

func NewWithStore(dir string, storeDir string) (*Catalog, error) {
	hashStore, err := NewHashStore(storeDir)
	if err != nil {
		return nil, err
	}
	catalog := &Catalog{dir: dir, hashStore: hashStore, readDir: os.ReadDir}
	if err := catalog.Refresh(); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (catalog *Catalog) List() ([]Model, error) {
	snapshot := catalog.snapshot.Load()
	if snapshot == nil {
		return nil, nil
	}
	if snapshot.loadErr != nil {
		return nil, snapshot.loadErr
	}
	return cloneModels(snapshot.models), nil
}

func (catalog *Catalog) Refresh() error {
	catalog.refreshMu.Lock()
	defer catalog.refreshMu.Unlock()

	if catalog.hashStore != nil {
		catalog.hashStore.StartScan()
	}
	models, err := catalog.scanModels()
	if err != nil {
		return err
	}
	if catalog.hashStore != nil {
		catalog.hashStore.FinishScan()
	}
	catalog.snapshot.Store(newCatalogSnapshot(models, nil))
	return nil
}

func (catalog *Catalog) Flush() error {
	if catalog == nil || catalog.hashStore == nil {
		return nil
	}
	return catalog.hashStore.Flush()
}

func (catalog *Catalog) Close() error {
	return catalog.Flush()
}

func (catalog *Catalog) scanModels() ([]Model, error) {
	readDir := catalog.readDir
	if readDir == nil {
		readDir = os.ReadDir
	}
	entries, err := readDir(catalog.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Model{}, nil
		}
		return nil, err
	}

	models := make([]Model, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".kcpps") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		filename := entry.Name()
		model := Model{
			ID:       strings.TrimSuffix(filename, filepath.Ext(filename)),
			Filename: filename,
			Path:     filepath.Join(catalog.dir, filename),
			Created:  info.ModTime().Unix(),
		}
		model = catalog.withMetadata(model)
		models = append(models, model)
	}

	sort.Slice(models, func(left, right int) bool {
		return models[left].ID < models[right].ID
	})
	return models, nil
}

func (catalog *Catalog) Resolve(id string) (Model, bool, error) {
	if id != filepath.Base(id) {
		return Model{}, false, nil
	}
	snapshot := catalog.snapshot.Load()
	if snapshot == nil {
		return Model{}, false, nil
	}
	if snapshot.loadErr != nil {
		return Model{}, false, snapshot.loadErr
	}
	model, ok := snapshot.byID[id]
	return cloneModel(model), ok, nil
}

func (catalog *Catalog) ListLLM() ([]Model, error) {
	models, err := catalog.List()
	if err != nil {
		return nil, err
	}
	filtered := make([]Model, 0, len(models))
	for _, model := range models {
		if model.HasLLM {
			filtered = append(filtered, model)
		}
	}
	return filtered, nil
}

func (catalog *Catalog) ListImages(activeConfigFilename string) ([]Model, error) {
	models, err := catalog.List()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	filtered := make([]Model, 0, len(models))
	for _, model := range models {
		if !model.HasImage {
			continue
		}
		if activeConfigFilename != AllImageConfigs && model.HasLLM && model.Filename != activeConfigFilename {
			continue
		}
		if _, ok := seen[model.ImageID]; ok {
			continue
		}
		seen[model.ImageID] = struct{}{}
		filtered = append(filtered, model)
	}
	return filtered, nil
}

func (catalog *Catalog) ResolveImage(id string, activeConfigFilename string) (Model, bool, error) {
	if id != filepath.Base(id) {
		return Model{}, false, nil
	}
	models, err := catalog.ListImages(activeConfigFilename)
	if err != nil {
		return Model{}, false, err
	}
	for _, model := range models {
		if model.ImageID == id {
			return model, true, nil
		}
	}
	return Model{}, false, nil
}

func (catalog *Catalog) ResolveActiveImage(activeConfigFilename string) (Model, bool, error) {
	if activeConfigFilename == "" || activeConfigFilename != filepath.Base(activeConfigFilename) {
		return Model{}, false, nil
	}
	models, err := catalog.ListImages(activeConfigFilename)
	if err != nil {
		return Model{}, false, err
	}
	for _, model := range models {
		if model.Filename == activeConfigFilename {
			return model, true, nil
		}
	}
	return Model{}, false, nil
}

func (catalog *Catalog) withMetadata(model Model) Model {
	model.HasLLM = true
	content, err := os.ReadFile(model.Path)
	if err != nil {
		model.Capabilities = capabilitiesFromMetadata(configMetadata{}, model.HasLLM, false, false, false, false, false)
		return model
	}
	var options map[string]json.RawMessage
	if err := json.Unmarshal(content, &options); err != nil {
		model.Capabilities = capabilitiesFromMetadata(configMetadata{}, model.HasLLM, false, false, false, false, false)
		return model
	}
	model.Options = options
	model.ChatTemplate = ChatTemplateProfileForConfig(content)
	var metadata configMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		model.Capabilities = capabilitiesFromMetadata(configMetadata{}, model.HasLLM, false, false, false, false, false)
		return model
	}

	model.HasImage = metadata.ImageModelPath() != ""
	model.HasEmbeddings = strings.TrimSpace(metadata.EmbeddingsModel) != ""
	model.HasMultimodal = modelHasValue(metadata.MMProj)
	model.HasVoice = hasVoiceModel(metadata)
	model.HasMusic = hasMusicModel(metadata)
	model.HasLLM = hasLLMModel(metadata)
	model.BackendMode = strings.TrimSpace(metadata.BackendMode)
	if model.HasImage {
		model.ImageModelPath = metadata.ImageModelPath()
		model.ImageModelName = filenameStem(model.ImageModelPath)
		model.ImageID = model.ID + "-" + model.ImageModelName
	}
	model.Capabilities = capabilitiesFromMetadata(metadata, model.HasLLM, model.HasImage, model.HasEmbeddings, model.HasMultimodal, model.HasVoice, model.HasMusic)
	model.ConfigHash = ConfigHash(content)
	if catalog.hashStore != nil {
		model.ModelHash = catalog.hashStore.ModelHash(content)
	} else {
		model.ModelHash = ModelReferenceHash(content, nil)
	}
	return model
}

func newCatalogSnapshot(models []Model, loadErr error) *catalogSnapshot {
	cloned := cloneModels(models)
	snapshot := &catalogSnapshot{
		models:  cloned,
		byID:    make(map[string]Model, len(cloned)),
		loadErr: loadErr,
	}
	for _, model := range cloned {
		snapshot.byID[model.ID] = model
	}
	return snapshot
}

func cloneModels(models []Model) []Model {
	cloned := make([]Model, len(models))
	for index := range models {
		cloned[index] = cloneModel(models[index])
	}
	return cloned
}

func cloneModel(model Model) Model {
	cloned := model
	if model.Options != nil {
		cloned.Options = make(map[string]json.RawMessage, len(model.Options))
		for key, value := range model.Options {
			cloned.Options[key] = append(json.RawMessage(nil), value...)
		}
	}
	cloned.Capabilities = cloneCapabilities(model.Capabilities)
	cloned.ChatTemplate = model.ChatTemplate.clone()
	return cloned
}

func cloneCapabilities(capabilities Capabilities) Capabilities {
	cloned := capabilities
	if capabilities.Image != nil {
		image := *capabilities.Image
		image.LoRA = append([]string{}, capabilities.Image.LoRA...)
		cloned.Image = &image
	}
	if capabilities.Embeddings != nil {
		embeddings := *capabilities.Embeddings
		cloned.Embeddings = &embeddings
	}
	if capabilities.Multimodal != nil {
		multimodal := *capabilities.Multimodal
		cloned.Multimodal = &multimodal
	}
	if capabilities.Voice != nil {
		voice := *capabilities.Voice
		cloned.Voice = &voice
	}
	if capabilities.Music != nil {
		music := *capabilities.Music
		cloned.Music = &music
	}
	return cloned
}

func hasLLMModel(metadata configMetadata) bool {
	if strings.TrimSpace(metadata.ModelParam) != "" {
		return true
	}
	if modelHasValue(metadata.Model) {
		return true
	}
	return !metadata.NoModel && strings.TrimSpace(metadata.SDModel) == ""
}

func hasVoiceModel(metadata configMetadata) bool {
	return strings.TrimSpace(metadata.WhisperModel) != "" ||
		strings.TrimSpace(metadata.TTSModel) != "" ||
		strings.TrimSpace(metadata.TTSWAVTokenizer) != "" ||
		strings.TrimSpace(metadata.TalkerModel) != "" ||
		strings.TrimSpace(metadata.Code2WAVModel) != "" ||
		strings.TrimSpace(metadata.TTSDir) != ""
}

func hasMusicModel(metadata configMetadata) bool {
	return strings.TrimSpace(metadata.MusicLLM) != "" ||
		strings.TrimSpace(metadata.MusicEmbeddings) != "" ||
		strings.TrimSpace(metadata.MusicDiffusion) != "" ||
		strings.TrimSpace(metadata.MusicVAE) != ""
}

func modelHasValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		for _, item := range typed {
			if modelHasValue(item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func filenameStem(value string) string {
	value = strings.TrimSpace(value)
	separator := strings.LastIndexAny(value, `/\`)
	if separator >= 0 {
		value = value[separator+1:]
	}
	extension := filepath.Ext(value)
	if extension == "" {
		return value
	}
	return strings.TrimSuffix(value, extension)
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, stringValues(item)...)
		}
		return values
	default:
		return nil
	}
}

func firstStringValue(value any) string {
	values := stringValues(value)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
