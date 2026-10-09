package modelassets

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxConfigReferenceBytes = 8 << 20

func (index *Index) IndexRoots(roots []string) error {
	paths := make([]string, 0)
	for _, root := range roots {
		files, err := regularFilesUnder(strings.TrimSpace(root), func(os.DirEntry) bool { return true })
		if err != nil {
			return err
		}
		paths = append(paths, files...)
	}
	if err := index.indexFilesConcurrently(paths); err != nil {
		return err
	}
	return index.Save()
}

func (index *Index) indexFilesConcurrently(paths []string) error {
	jobs := make(chan string)
	errors := make(chan error, index.hashWorkers)
	var workers sync.WaitGroup
	for range index.hashWorkers {
		workers.Go(func() {
			for path := range jobs {
				if _, err := index.indexFile(path); err != nil {
					select {
					case errors <- err:
					default:
					}
				}
			}
		})
	}
	for _, path := range paths {
		jobs <- path
	}
	close(jobs)
	workers.Wait()
	close(errors)
	return <-errors
}

func regularFilesUnder(root string, accept func(os.DirEntry) bool) ([]string, error) {
	if root == "" {
		return nil, nil
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && entry.Type().IsRegular() && accept(entry) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func (index *Index) IndexConfigReferences(configDir string) error {
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.WalkDir(configDir, func(configPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Ext(entry.Name()), ".kcpps") {
			return nil
		}
		index.indexReferencedModelFiles(configPath)
		return nil
	})
}

func (index *Index) indexReferencedModelFiles(configPath string) {
	file, err := os.Open(configPath)
	if err != nil {
		return
	}
	var config map[string]any
	decodeErr := json.NewDecoder(io.LimitReader(file, maxConfigReferenceBytes)).Decode(&config)
	_ = file.Close()
	if decodeErr != nil {
		return
	}
	for field, value := range config {
		if !isModelField(field) {
			continue
		}
		paths, _, ok := pathValues(value)
		if !ok {
			continue
		}
		for _, modelPath := range paths {
			_, _ = index.IndexFile(modelPath)
		}
	}
}

func (index *Index) FindInRoots(hash string, filename string, roots []string) (string, bool, error) {
	if path, found := index.Find(hash, filename); found {
		return path, true, nil
	}
	if !validHash(hash) || !safeFilename(filename) {
		return "", false, nil
	}
	candidates, err := index.filesNamedUnderRoots(filename, roots)
	if err != nil {
		return "", false, err
	}
	for _, candidate := range candidates {
		asset, err := index.IndexFile(candidate)
		if err != nil {
			return "", false, err
		}
		if asset.SHA256 == hash {
			return asset.Path, true, nil
		}
	}
	return "", false, nil
}

func (index *Index) filesNamedUnderRoots(filename string, roots []string) ([]string, error) {
	candidates := make([]string, 0)
	seenRoots := map[string]struct{}{}
	for _, root := range append(append([]string{}, roots...), index.sharedDir) {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		absoluteRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		if _, seen := seenRoots[absoluteRoot]; seen {
			continue
		}
		seenRoots[absoluteRoot] = struct{}{}
		files, err := regularFilesUnder(absoluteRoot, func(entry os.DirEntry) bool { return entry.Name() == filename })
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, files...)
	}
	return candidates, nil
}
