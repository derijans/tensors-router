package cluster

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Store struct {
	path string
}

func NewStore(storeDir string) (*Store, error) {
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(storeDir, "registry.json")}, nil
}

func (store *Store) Save(snapshot Snapshot) error {
	content, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(store.path, content, 0o644)
}
