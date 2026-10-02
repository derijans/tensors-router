package downloader

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const finishedJobRetention = 30 * 24 * time.Hour

func (manager *Manager) RecoverInterrupted() {
	manager.restoreInterruptedPromotions()
	if pruned, err := manager.store.PruneFinishedJobs(time.Now().Add(-finishedJobRetention)); err != nil {
		manager.logStartup("downloader could not prune finished jobs: %v", err)
	} else if pruned > 0 {
		manager.logStartup("downloader pruned %d finished job(s) older than %s", pruned, finishedJobRetention)
	}
	manager.removeOrphanedStaging()
	interrupted, err := manager.store.RequeueInterrupted()
	if err != nil {
		manager.logStartup("downloader could not requeue interrupted jobs: %v", err)
		return
	}
	for _, jobID := range interrupted {
		if err := manager.startJob(jobID); err != nil {
			manager.logStartup("downloader could not resume job=%s: %v", jobID, err)
			continue
		}
		manager.logStartup("downloader resuming interrupted job=%s", jobID)
	}
}

func (manager *Manager) restoreInterruptedPromotions() {
	_ = filepath.WalkDir(manager.config.Storage.Root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") && strings.Contains(name, promotionFileMarker) {
			manager.removeLeftover(path)
			return nil
		}
		marker := strings.LastIndex(name, replacedFileMarker)
		if marker <= 0 {
			return nil
		}
		original := filepath.Join(filepath.Dir(path), name[:marker])
		if _, err := os.Lstat(original); os.IsNotExist(err) {
			if err := renameWithRetry(path, original); err != nil {
				manager.logStartup("downloader could not restore %q: %v", original, err)
			} else {
				manager.logStartup("downloader restored %q after an interrupted replacement", original)
			}
			return nil
		}
		manager.removeLeftover(path)
		return nil
	})
}

func (manager *Manager) removeLeftover(path string) {
	if err := os.Remove(path); err != nil {
		manager.logStartup("downloader could not remove leftover %q: %v", path, err)
	}
}

func (manager *Manager) removeOrphanedStaging() {
	entries, err := os.ReadDir(filepath.Join(manager.config.Storage.StateDir, "staging"))
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		job, found, err := manager.store.Job(entry.Name())
		if err != nil {
			continue
		}
		if !found || job.State == JobCompleted || job.State == JobCancelled {
			manager.removeStaging(entry.Name())
		}
	}
}

func isDownloaderLeftover(name string) bool {
	return strings.Contains(name, promotionFileMarker) || strings.Contains(name, replacedFileMarker)
}
