package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func (manager *Manager) Rescan() ([]ArtifactRecord, error) {
	artifacts := []ArtifactRecord{}
	unhashed := []string{}
	err := filepath.WalkDir(manager.config.Storage.Root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasSuffix(entry.Name(), ".hash") || isDownloaderLeftover(entry.Name()) {
			return nil
		}
		record, known, err := manager.knownArtifact(filePath, entry)
		if err != nil {
			return err
		}
		if known {
			artifacts = append(artifacts, record)
		} else {
			unhashed = append(unhashed, filePath)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	hashed, err := manager.hashArtifacts(unhashed)
	return append(artifacts, hashed...), err
}

func (manager *Manager) knownArtifact(filePath string, entry os.DirEntry) (ArtifactRecord, bool, error) {
	if hash, trusted, err := ReadTrustedHashSidecar(filePath); err != nil {
		return ArtifactRecord{}, false, err
	} else if trusted {
		record, err := artifactFromFile(filePath, hash, "", "", "", "sidecar")
		if err != nil {
			return ArtifactRecord{}, false, err
		}
		return record, true, manager.recordArtifact(record)
	}
	info, err := entry.Info()
	if err != nil {
		return ArtifactRecord{}, false, err
	}
	record, found, err := manager.store.Artifact(filePath)
	if err != nil || !found || record.Size != info.Size() || record.ModifiedUnixNano != info.ModTime().UnixNano() {
		return ArtifactRecord{}, false, err
	}
	return record, true, manager.notifyArtifact(record)
}

func (manager *Manager) hashArtifacts(paths []string) ([]ArtifactRecord, error) {
	var mu sync.Mutex
	records := make([]ArtifactRecord, 0, len(paths))
	err := forEachBounded(context.Background(), manager.config.Scanning.HashWorkers, paths, func(_ context.Context, filePath string) error {
		hash, _, err := SHA256File(filePath)
		if err != nil {
			return err
		}
		record, err := artifactFromFile(filePath, hash, "", "", "", "scan")
		if err != nil {
			return err
		}
		if err := manager.recordArtifact(record); err != nil {
			return err
		}
		mu.Lock()
		records = append(records, record)
		mu.Unlock()
		return nil
	})
	return records, err
}
