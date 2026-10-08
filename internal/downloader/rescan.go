package downloader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func (manager *Manager) Rescan() ([]ArtifactRecord, error) {
	manager.scanMu.Lock()
	defer manager.scanMu.Unlock()
	artifacts, unknown, err := manager.scanLibrary(context.Background())
	if err != nil {
		return nil, err
	}
	manager.library.replace(unknown)
	hashed, err := manager.hashArtifacts(unknown)
	return append(artifacts, hashed...), err
}

func (manager *Manager) Unhashed() ([]UnhashedFile, error) {
	return manager.library.list(), nil
}

func (manager *Manager) scanLibrary(ctx context.Context) ([]ArtifactRecord, []UnhashedFile, error) {
	artifacts := []ArtifactRecord{}
	unknown := []UnhashedFile{}
	err := filepath.WalkDir(manager.config.Storage.Root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasSuffix(entry.Name(), ".hash") || isDownloaderLeftover(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		record, known, err := manager.knownArtifact(filePath, info)
		if err != nil {
			return err
		}
		if known {
			artifacts = append(artifacts, record)
		} else {
			unknown = append(unknown, UnhashedFile{Path: filePath, Size: info.Size()})
		}
		return nil
	})
	return artifacts, unknown, err
}

func (manager *Manager) knownArtifact(filePath string, info os.FileInfo) (ArtifactRecord, bool, error) {
	record, found, err := manager.store.Artifact(filePath)
	if err != nil {
		return ArtifactRecord{}, false, err
	}
	if found && record.describes(info) {
		return record, true, manager.notifyArtifact(record)
	}
	hash, trusted, err := ReadTrustedHashSidecar(filePath)
	if err != nil || !trusted {
		return ArtifactRecord{}, false, err
	}
	sidecarRecord, err := artifactFromFile(filePath, hash, "", "", "", "sidecar")
	if err != nil {
		return ArtifactRecord{}, false, err
	}
	saved, err := manager.recordArtifact(sidecarRecord)
	return saved, true, err
}

func (record ArtifactRecord) describes(info os.FileInfo) bool {
	return record.Size == info.Size() && record.ModifiedUnixNano == info.ModTime().UnixNano()
}

func (manager *Manager) hashArtifacts(files []UnhashedFile) ([]ArtifactRecord, error) {
	var mu sync.Mutex
	records := make([]ArtifactRecord, 0, len(files))
	err := forEachBounded(context.Background(), manager.config.Scanning.HashWorkers, files, func(_ context.Context, file UnhashedFile) error {
		hash, _, err := SHA256File(file.Path)
		if err != nil {
			return err
		}
		record, err := artifactFromFile(file.Path, hash, "", "", "", "scan")
		if err != nil {
			return err
		}
		saved, err := manager.recordArtifact(record)
		if err != nil {
			return err
		}
		mu.Lock()
		records = append(records, saved)
		mu.Unlock()
		return nil
	})
	return records, err
}
