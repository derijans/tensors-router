package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Catalog struct {
	dir string
}

type Model struct {
	ID             string
	Filename       string
	Path           string
	Created        int64
	HasLLM         bool
	HasImage       bool
	ImageID        string
	ImageModelName string
	ImageModelPath string
}

type configMetadata struct {
	Model      any    `json:"model"`
	ModelParam string `json:"model_param"`
	NoModel    bool   `json:"nomodel"`
	SDModel    string `json:"sdmodel"`
}

func New(dir string) *Catalog {
	return &Catalog{dir: dir}
}

func (catalog *Catalog) List() ([]Model, error) {
	entries, err := os.ReadDir(catalog.dir)
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
		if model.HasLLM && model.Filename != activeConfigFilename {
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
		return model
	}
	var metadata configMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return model
	}

	model.HasImage = strings.TrimSpace(metadata.SDModel) != ""
	if model.HasImage {
		model.ImageModelPath = strings.TrimSpace(metadata.SDModel)
		model.ImageModelName = filenameStem(model.ImageModelPath)
		model.ImageID = model.ID + "-" + model.ImageModelName
		model.HasLLM = hasLLMModel(metadata)
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
