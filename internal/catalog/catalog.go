package catalog

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Catalog struct {
	dir string
}

type Model struct {
	ID       string
	Filename string
	Path     string
	Created  int64
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
		models = append(models, Model{
			ID:       strings.TrimSuffix(filename, filepath.Ext(filename)),
			Filename: filename,
			Path:     filepath.Join(catalog.dir, filename),
			Created:  info.ModTime().Unix(),
		})
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
