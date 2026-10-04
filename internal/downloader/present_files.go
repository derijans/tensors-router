package downloader

import (
	"os"
	"strings"
)

const alreadyPresentReason = "already present"

func (manager *Manager) withoutPresentFiles(plan DownloadPlan) (DownloadPlan, error) {
	if plan.Snapshot {
		return plan, nil
	}
	remaining := make([]PlannedFile, 0, len(plan.Files))
	for _, file := range plan.Files {
		present, err := manager.filePresent(plan, file)
		if err != nil {
			return DownloadPlan{}, err
		}
		if !present {
			remaining = append(remaining, file)
			continue
		}
		plan.Skipped = append(plan.Skipped, SkippedFile{Path: file.Path, Reason: alreadyPresentReason})
		plan.TotalBytes -= file.Size
	}
	plan.Files = remaining
	return plan, nil
}

func (manager *Manager) filePresent(plan DownloadPlan, file PlannedFile) (bool, error) {
	if !validSHA256(file.LFSHash) {
		return false, nil
	}
	destination, err := downloadDestinationResolve(manager.config.Storage.Root, plan.Repository, plan.Commit, false, file.Path)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(destination)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil || info.Size() != file.Size {
		return false, err
	}
	hash, verified, err := manager.verifiedHash(destination, info)
	return verified && strings.EqualFold(hash, file.LFSHash), err
}

func (manager *Manager) verifiedHash(path string, info os.FileInfo) (string, bool, error) {
	if hash, trusted, err := ReadTrustedHashSidecar(path); err != nil || trusted {
		return hash, trusted, err
	}
	record, found, err := manager.store.Artifact(path)
	if err != nil || !found || !record.describes(info) {
		return "", false, err
	}
	return record.SHA256, true, nil
}

func (plan DownloadPlan) AlreadyPresent() bool {
	if len(plan.Files) > 0 {
		return false
	}
	for _, skipped := range plan.Skipped {
		if skipped.Reason == alreadyPresentReason {
			return true
		}
	}
	return false
}
