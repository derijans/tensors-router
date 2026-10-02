package downloader

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
)

type jobRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (manager *Manager) startJob(jobID string) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return fmt.Errorf("downloader manager is closed")
	}
	previous := manager.running[jobID]
	ctx, cancel := context.WithCancel(manager.lifecycle)
	run := &jobRun{cancel: cancel, done: make(chan struct{})}
	manager.running[jobID] = run
	manager.jobs.Add(1)
	go func() {
		defer manager.jobs.Done()
		defer close(run.done)
		defer cancel()
		if previous != nil {
			<-previous.done
		}
		manager.run(ctx, jobID)
		manager.finishRun(jobID, run)
	}()
	return nil
}

func (manager *Manager) cancelRun(jobID string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if run := manager.running[jobID]; run != nil {
		run.cancel()
	}
}

func (manager *Manager) finishRun(jobID string, run *jobRun) {
	job, found, _ := manager.store.Job(jobID)
	manager.mu.Lock()
	if manager.running[jobID] == run {
		delete(manager.running, jobID)
	}
	if !found || job.State == JobCompleted || job.State == JobCancelled {
		delete(manager.tokens, jobID)
	}
	manager.mu.Unlock()
	if found && job.State == JobCancelled {
		manager.removeStaging(jobID)
	}
}

func (manager *Manager) run(ctx context.Context, jobID string) {
	select {
	case manager.semaphore <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-manager.semaphore }()
	started, err := manager.store.TransitionJob(jobID, JobRunning, "", JobQueued)
	if err != nil {
		manager.logRuntime("download could not start job=%s error=%q", jobID, err)
		return
	}
	if !started {
		return
	}
	job, err := manager.currentJob(jobID)
	if err != nil {
		manager.failRunningJob(jobID, fmt.Errorf("read job before starting: %w", err))
		return
	}
	manager.publish(job)
	manager.logRuntime("download started job=%s repository=%q", job.ID, job.Repository)
	if err := manager.transfer(ctx, job); err != nil {
		if manager.lifecycle.Err() == nil {
			manager.failRunningJob(jobID, err)
		}
		return
	}
	if completed, err := manager.store.TransitionJob(jobID, JobCompleted, "", JobRunning); err != nil || !completed {
		return
	}
	if err := manager.store.CompleteAllFiles(jobID); err != nil {
		manager.logRuntime("download completed job=%s but file states could not be saved: %v", jobID, err)
	}
	manager.publishCurrent(jobID)
	manager.logRuntime("download completed job=%s repository=%q bytes=%d", job.ID, job.Repository, job.TotalBytes)
}

func (manager *Manager) failRunningJob(jobID string, cause error) {
	message := redactSensitive(cause.Error())
	failed, err := manager.store.TransitionJob(jobID, JobFailed, message, JobRunning)
	if err != nil || !failed {
		return
	}
	if err := manager.store.FailUnfinishedFiles(jobID, message); err != nil {
		manager.logRuntime("download failed job=%s but file states could not be saved: %v", jobID, err)
	}
	manager.publishCurrent(jobID)
	manager.logRuntime("download failed job=%s error=%q", jobID, message)
}

func (manager *Manager) transfer(ctx context.Context, job DownloadJob) error {
	staging, err := manager.stagingDirectory(job.ID)
	if err != nil {
		return err
	}
	pending := make([]JobFile, 0, len(job.Files))
	for _, file := range job.Files {
		if file.State != string(JobCompleted) {
			pending = append(pending, file)
		}
	}
	release, err := manager.reserveSpace(missingBytes(staging, pending))
	if err != nil {
		return err
	}
	defer release()
	if err := forEachBounded(ctx, manager.config.Downloads.ConcurrentFiles, pending, func(ctx context.Context, file JobFile) error {
		return manager.transferFile(ctx, job, staging, file)
	}); err != nil {
		return err
	}
	if job.Snapshot {
		treeDigest, err := computeDownloadedSnapshotDigest(manager.config.Storage.Root, job.Repository, job.Commit, job.Files)
		if err != nil {
			return err
		}
		if err := manager.store.SetTreeDigest(job.ID, treeDigest); err != nil {
			return err
		}
	}
	return os.RemoveAll(staging)
}

func (manager *Manager) transferFile(ctx context.Context, job DownloadJob, staging string, file JobFile) error {
	stagedPath, err := secureStagingPath(staging, file.Path)
	if err != nil {
		return err
	}
	if err := manager.store.UpdateFile(job.ID, file.Path, JobRunning, "", stagedSize(stagedPath)); err != nil {
		return err
	}
	manager.publishCurrent(job.ID)
	reporter := &fileProgressReporter{manager: manager, jobID: job.ID, path: file.Path}
	digests, err := manager.downloadFile(ctx, fileDownload{
		repository:     job.Repository,
		commit:         job.Commit,
		repositoryPath: file.Path,
		stagingPath:    stagedPath,
		expectedSize:   file.Size,
		token:          manager.jobToken(job.ID),
		onProgress:     reporter.report,
	})
	if err != nil {
		return err
	}
	if err := verifyStagedFile(file, digests); err != nil {
		_ = os.Remove(stagedPath)
		return err
	}
	if err := manager.promote(job, file, stagedPath, digests.sha256); err != nil {
		return err
	}
	if err := manager.store.UpdateFile(job.ID, file.Path, JobCompleted, "", digests.size); err != nil {
		return err
	}
	manager.publishCurrent(job.ID)
	return nil
}

func verifyStagedFile(file JobFile, digests stagedDigests) error {
	if digests.size != file.Size {
		return fmt.Errorf("downloaded size for %q differs from the planned size", file.Path)
	}
	if expected := strings.TrimPrefix(file.ExpectedSHA256, "sha256:"); validSHA256(expected) {
		if digests.sha256 != expected {
			return fmt.Errorf("downloaded hash for %q does not match the remote LFS SHA-256", file.Path)
		}
		return nil
	}
	if expected := strings.ToLower(file.ExpectedGitOID); validGitObjectID(expected) && digests.gitBlobSHA1 != expected {
		return fmt.Errorf("downloaded content for %q does not match the remote git blob id", file.Path)
	}
	return nil
}

func validGitObjectID(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func missingBytes(staging string, files []JobFile) int64 {
	var missing int64
	for _, file := range files {
		stagedPath, err := secureStagingPath(staging, file.Path)
		if err != nil {
			missing += file.Size
			continue
		}
		missing += max(file.Size-stagedSize(stagedPath), 0)
	}
	return missing
}

func forEachBounded[T any](ctx context.Context, limit int, items []T, work func(context.Context, T) error) error {
	workContext, cancel := context.WithCancel(ctx)
	defer cancel()
	slots := make(chan struct{}, max(limit, 1))
	var workers sync.WaitGroup
	var failure sync.Once
	var firstError error
	for _, item := range items {
		select {
		case slots <- struct{}{}:
		case <-workContext.Done():
		}
		if workContext.Err() != nil {
			break
		}
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { <-slots }()
			if err := work(workContext, item); err != nil {
				failure.Do(func() {
					firstError = err
					cancel()
				})
			}
		}()
	}
	workers.Wait()
	if firstError != nil {
		return firstError
	}
	return ctx.Err()
}
