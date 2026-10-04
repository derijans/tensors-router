package downloader

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	replacedFileMarker  = ".tensor-router-replaced-"
	promotionFileMarker = ".tensor-router-promote-"
)

func (manager *Manager) promote(job DownloadJob, file JobFile, stagedPath string, hash string) error {
	destination, err := downloadDestinationPath(manager.config.Storage.Root, job.Repository, job.Commit, job.Snapshot, file.Path)
	if err != nil {
		return err
	}
	if err := ensureDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	if job.Snapshot {
		identical, err := snapshotDestinationMatches(destination, hash, file.Size)
		if err != nil || identical {
			return err
		}
	}
	temporary, err := preparePromotionFile(job.ID, stagedPath, destination, hash)
	if err != nil {
		return err
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporary)
		}
	}()
	backup := ""
	if info, err := os.Lstat(destination); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination is a symbolic link")
		}
		backup = destination + replacedFileMarker + job.ID
		if err := renameWithRetry(destination, backup); err != nil {
			return fmt.Errorf("move existing %q aside (is it open in a running backend?): %w", filepath.Base(destination), err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := renameWithRetry(temporary, destination); err != nil {
		if backup != "" {
			_ = renameWithRetry(backup, destination)
		}
		return err
	}
	removeTemporary = false
	if backup != "" {
		_ = os.Remove(backup)
	}
	record, err := artifactFromFile(destination, hash, job.Repository, file.Path, job.Commit, "download")
	if err != nil {
		return err
	}
	if _, err := manager.recordArtifact(record); err != nil {
		return err
	}
	if manager.config.Scanning.WriteHashSidecars && !job.Snapshot {
		return WriteHashSidecar(destination, hash)
	}
	return nil
}

func snapshotDestinationMatches(destination string, hash string, size int64) (bool, error) {
	info, err := os.Lstat(destination)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return false, fmt.Errorf("immutable snapshot destination is not a regular file")
	}
	existingHash, existingSize, err := SHA256File(destination)
	if err != nil {
		return false, err
	}
	if existingHash != hash || existingSize != size {
		return false, fmt.Errorf("immutable snapshot destination already exists with different content")
	}
	return true, nil
}

func preparePromotionFile(jobID string, stagedPath string, destination string, expectedHash string) (string, error) {
	if !safeRepositoryPart(jobID) {
		return "", fmt.Errorf("download job ID is invalid")
	}
	temporaryPath := filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+promotionFileMarker+jobID)
	if _, err := os.Lstat(temporaryPath); err == nil {
		if err := os.Remove(temporaryPath); err != nil {
			return "", fmt.Errorf("remove stale promotion file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := renameWithRetry(stagedPath, temporaryPath); err == nil {
		return temporaryPath, nil
	}
	return copyPromotionFile(stagedPath, temporaryPath, expectedHash)
}

func copyPromotionFile(stagedPath string, temporaryPath string, expectedHash string) (string, error) {
	info, err := os.Lstat(stagedPath)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("staged download is not a regular file")
	}
	source, err := os.Open(stagedPath)
	if err != nil {
		return "", err
	}
	defer source.Close()
	temporary, err := os.OpenFile(temporaryPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := io.Copy(temporary, source); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	copiedHash, copiedSize, err := SHA256File(temporaryPath)
	if err != nil {
		return "", err
	}
	if copiedHash != expectedHash || copiedSize != info.Size() {
		return "", fmt.Errorf("copied download differs from verified staging file")
	}
	removeTemporary = false
	return temporaryPath, nil
}
