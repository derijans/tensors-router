package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Catalog struct {
	scanMu    sync.Mutex
	dir       string
	hashStore *HashStore
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
	ImageID        string
	ImageModelName string
	ImageModelPath string
	ModelHash      string
	ConfigHash     string
	Capabilities   Capabilities
}

func New(dir string) *Catalog {
	return &Catalog{dir: dir}
}

func NewWithStore(dir string, storeDir string) (*Catalog, error) {
	hashStore, err := NewHashStore(storeDir)
	if err != nil {
		return nil, err
	}
	return &Catalog{dir: dir, hashStore: hashStore}, nil
}

func (catalog *Catalog) List() ([]Model, error) {
	catalog.scanMu.Lock()
	defer catalog.scanMu.Unlock()

	if catalog.hashStore != nil {
		catalog.hashStore.StartScan()
	}
	entries, err := os.ReadDir(catalog.dir)
	if err != nil {
		if os.IsNotExist(err) {
			if catalog.hashStore != nil {
				if saveErr := catalog.hashStore.Save(); saveErr != nil {
					return nil, saveErr
				}
			}
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
	if catalog.hashStore != nil {
		if err := catalog.hashStore.Save(); err != nil {
			return nil, err
		}
	}
	return models, nil
}

func (catalog *Catalog) Resolve(id string) (Model, bool, error) {
	if id != filepath.Base(id) {
		return Model{}, false, nil
	}
	models, err := catalog.List()
	if err != nil {
		return Model{}, false, err
	}
	for _, model := range models {
		if model.ID == id {
			return model, true, nil
		}
	}
	return Model{}, false, nil
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
		model.Capabilities = capabilitiesFromMetadata(configMetadata{}, model.HasLLM, false, false, false)
		return model
	}
	var metadata configMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		model.Capabilities = capabilitiesFromMetadata(configMetadata{}, model.HasLLM, false, false, false)
		return model
	}

	model.HasImage = strings.TrimSpace(metadata.SDModel) != ""
	model.HasEmbeddings = strings.TrimSpace(metadata.EmbeddingsModel) != ""
	model.HasMultimodal = modelHasValue(metadata.MMProj)
	model.HasLLM = hasLLMModel(metadata)
	if model.HasImage {
		model.ImageModelPath = strings.TrimSpace(metadata.SDModel)
		model.ImageModelName = filenameStem(model.ImageModelPath)
		model.ImageID = model.ID + "-" + model.ImageModelName
	}
	model.Capabilities = capabilitiesFromMetadata(metadata, model.HasLLM, model.HasImage, model.HasEmbeddings, model.HasMultimodal)
	model.ConfigHash = ConfigHash(content)
	if catalog.hashStore != nil {
		model.ModelHash = catalog.hashStore.ModelHash(content)
	} else {
		model.ModelHash = ModelReferenceHash(content, nil)
	}
	return model
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
