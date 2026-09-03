package downloader

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Manager struct {
	config          Config
	store           *Store
	hub             *HubClient
	lifecycle       context.Context
	stopLifecycle   context.CancelFunc
	mu              sync.Mutex
	running         map[string]context.CancelFunc
	tokens          map[string]string
	subscribers     map[string]map[chan DownloadJob]struct{}
	semaphore       chan struct{}
	artifactHandler func(ArtifactRecord) error
	logger          *log.Logger
	logFile         *os.File
	jobs            sync.WaitGroup
	closed          bool
	closeOnce       sync.Once
	closeError      error
}

type ArtifactHandler func(ArtifactRecord) error

func NewManager(config Config, _ string) (*Manager, error) {
	if err := ensureDirectory(config.Storage.Root); err != nil {
		return nil, fmt.Errorf("initialize downloader storage root: %w", err)
	}
	if err := ensureDirectory(config.Storage.StateDir); err != nil {
		return nil, fmt.Errorf("initialize downloader state storage: %w", err)
	}
	logger, logFile, err := newManagerLogger(config.Logging)
	if err != nil {
		return nil, fmt.Errorf("initialize downloader logging: %w", err)
	}
	store, err := OpenStore(config.Storage.DatabasePath)
	if err != nil {
		_ = closeManagerLog(logFile)
		return nil, fmt.Errorf("initialize downloader database: %w", err)
	}
	recovered, recoverErr := store.RecoverInterrupted("downloader stopped before the download finished")
	if recoverErr != nil {
		logger.Printf("downloader could not recover interrupted jobs: %v", recoverErr)
	} else if recovered > 0 {
		logger.Printf("downloader marked %d interrupted job(s) as failed", recovered)
	}
	lifecycle, stopLifecycle := context.WithCancel(context.Background())
	manager := &Manager{config: config, store: store, hub: NewHubClient(config.Downloads.Timeout), lifecycle: lifecycle, stopLifecycle: stopLifecycle, running: map[string]context.CancelFunc{}, tokens: map[string]string{}, subscribers: map[string]map[chan DownloadJob]struct{}{}, semaphore: make(chan struct{}, config.Downloads.ConcurrentJobs), logger: logger, logFile: logFile}
	manager.logStartup("downloader initialized storage=%q", config.Storage.Root)
	return manager, nil
}

func (manager *Manager) Close() error {
	manager.closeOnce.Do(func() {
		manager.mu.Lock()
		manager.closed = true
		manager.stopLifecycle()
		for _, cancel := range manager.running {
			cancel()
		}
		manager.mu.Unlock()
		manager.jobs.Wait()
		manager.closeError = errors.Join(manager.store.Close(), closeManagerLog(manager.logFile))
	})
	return manager.closeError
}

func (manager *Manager) SetArtifactHandler(handler ArtifactHandler) {
	manager.mu.Lock()
	manager.artifactHandler = handler
	manager.mu.Unlock()
}

func (manager *Manager) Capability() Capability {
	capability := Capability{Available: true, Configured: true, ConfiguredToken: strings.TrimSpace(manager.config.HuggingFace.Token) != "", StorageRoot: manager.config.Storage.Root, FreeSpaceReserveBytes: manager.config.Storage.FreeSpaceReserveGB << 30}
	if bytes, known, err := availableSpace(manager.config.Storage.Root); err != nil {
		capability.Error = fmt.Sprintf("inspect downloader storage capacity: %v", err)
		capability.Reason = capability.Error
	} else if known {
		capability.FreeBytes = bytes
	}
	return capability
}

func MergeRuntimeCapability(startup Capability, runtime Capability) Capability {
	runtime.Enabled = startup.Enabled
	runtime.Present = startup.Present
	runtime.Working = startup.Working
	if runtime.Error != "" {
		runtime.Working = false
		runtime.Reason = runtime.Error
	} else if !runtime.Working {
		runtime.Reason = startup.Reason
		runtime.Error = startup.Error
	}
	return runtime
}

func (manager *Manager) Search(ctx context.Context, request SearchRequest, operationToken string) ([]SearchResult, error) {
	return manager.hub.Search(ctx, request, manager.token(operationToken))
}

func (manager *Manager) SearchPage(ctx context.Context, request SearchRequest, operationToken string) (SearchPage, error) {
	return manager.hub.SearchPage(ctx, request, manager.token(operationToken))
}

func (manager *Manager) Repository(ctx context.Context, request RepositoryRequest) (RepositoryDetails, error) {
	return manager.hub.Repository(ctx, request.Repository, request.Revision, manager.token(request.Token))
}

func (manager *Manager) Plan(ctx context.Context, request PlanRequest) (DownloadPlan, error) {
	details, err := manager.hub.Repository(ctx, request.Repository, request.Revision, manager.token(request.Token))
	if err != nil {
		return DownloadPlan{}, err
	}
	return BuildPlan(details, request.Files, request.Mode, manager.config.Storage.Root)
}

func (manager *Manager) CreateJob(ctx context.Context, request CreateJobRequest) (DownloadJob, error) {
	mode := strings.TrimSpace(request.Mode)
	if mode == "" {
		mode = "smart"
	}
	if mode != "smart" && mode != "explicit" && mode != "snapshot" {
		return DownloadJob{}, fmt.Errorf("download mode must be smart, explicit, or snapshot")
	}
	plan, err := manager.Plan(ctx, PlanRequest{Repository: request.Repository, Revision: request.Revision, Files: request.Files, Mode: mode, Token: request.Token})
	if err != nil {
		return DownloadJob{}, err
	}
	return manager.CreatePlannedJob(plan, request.Token, request.ConfirmUnsafe, request.ConfirmReplace)
}

func (manager *Manager) CreatePlannedJob(plan DownloadPlan, operationToken string, confirmUnsafe bool, confirmReplace bool) (DownloadJob, error) {
	if plan.UnsafeWarning && !confirmUnsafe {
		return DownloadJob{}, fmt.Errorf("repository has an unsafe or pending security status; explicit confirmation is required")
	}
	if err := manager.ensureFreeSpace(plan.TotalBytes); err != nil {
		return DownloadJob{}, err
	}
	if err := manager.ensureReplacementAllowed(plan, confirmReplace); err != nil {
		return DownloadJob{}, err
	}
	if len(plan.Files) == 0 {
		return DownloadJob{}, fmt.Errorf("download plan resolved to zero files; the repository, revision, or file selection matched nothing on Hugging Face")
	}
	job := DownloadJob{ID: randomJobID(), Repository: plan.Repository, Revision: plan.Revision, Commit: plan.Commit, State: JobQueued, TotalBytes: plan.TotalBytes, Snapshot: plan.Snapshot, Files: make([]JobFile, 0, len(plan.Files))}
	for _, file := range plan.Files {
		job.Files = append(job.Files, JobFile{Path: file.Path, Reason: file.Reason, ExpectedSHA256: file.LFSHash, Size: file.Size, State: string(JobQueued)})
	}
	if err := manager.store.SaveJob(job); err != nil {
		return DownloadJob{}, err
	}
	stored, _, err := manager.store.Job(job.ID)
	if err != nil {
		return DownloadJob{}, err
	}
	manager.mu.Lock()
	if token := strings.TrimSpace(operationToken); token != "" {
		manager.tokens[job.ID] = token
	}
	manager.mu.Unlock()
	manager.logRuntime("download queued job=%s repository=%q files=%d bytes=%d", stored.ID, stored.Repository, len(stored.Files), stored.TotalBytes)
	if err := manager.startJob(stored); err != nil {
		return DownloadJob{}, err
	}
	return stored, nil
}

func (manager *Manager) Job(id string) (DownloadJob, bool, error) { return manager.store.Job(id) }

func (manager *Manager) Jobs() ([]DownloadJob, error) { return manager.store.Jobs() }

func (manager *Manager) Artifacts() ([]ArtifactRecord, error) { return manager.store.ListArtifacts() }

func (manager *Manager) Pause(id string) (DownloadJob, error) {
	job, found, err := manager.store.Job(id)
	if err != nil {
		return DownloadJob{}, err
	}
	if !found {
		return DownloadJob{}, fmt.Errorf("download job was not found")
	}
	if job.State != JobQueued && job.State != JobRunning {
		return DownloadJob{}, fmt.Errorf("download job cannot be paused from %s", job.State)
	}
	job.State = JobPaused
	if err := manager.store.SaveJob(job); err != nil {
		return DownloadJob{}, err
	}
	manager.mu.Lock()
	if cancel := manager.running[id]; cancel != nil {
		cancel()
	}
	manager.mu.Unlock()
	manager.publish(job)
	manager.logRuntime("download paused job=%s repository=%q", job.ID, job.Repository)
	return manager.currentJob(id)
}

func (manager *Manager) Resume(id string) (DownloadJob, error) {
	job, found, err := manager.store.Job(id)
	if err != nil {
		return DownloadJob{}, err
	}
	if !found {
		return DownloadJob{}, fmt.Errorf("download job was not found")
	}
	if job.State != JobPaused && job.State != JobFailed {
		return DownloadJob{}, fmt.Errorf("download job cannot be resumed from %s", job.State)
	}
	job.State, job.Error = JobQueued, ""
	for index := range job.Files {
		if job.Files[index].State != string(JobCompleted) {
			job.Files[index].State, job.Files[index].Error = string(JobQueued), ""
		}
	}
	if err := manager.store.SaveJob(job); err != nil {
		return DownloadJob{}, err
	}
	stored, _, err := manager.store.Job(id)
	if err != nil {
		return DownloadJob{}, err
	}
	manager.logRuntime("download resumed job=%s repository=%q", stored.ID, stored.Repository)
	if err := manager.startJob(stored); err != nil {
		return DownloadJob{}, err
	}
	return stored, nil
}

func (manager *Manager) Cancel(id string) (DownloadJob, error) {
	job, found, err := manager.store.Job(id)
	if err != nil {
		return DownloadJob{}, err
	}
	if !found {
		return DownloadJob{}, fmt.Errorf("download job was not found")
	}
	if job.State == JobCompleted || job.State == JobCancelled {
		return DownloadJob{}, fmt.Errorf("download job cannot be cancelled from %s", job.State)
	}
	job.State = JobCancelled
	if err := manager.store.SaveJob(job); err != nil {
		return DownloadJob{}, err
	}
	manager.mu.Lock()
	if cancel := manager.running[id]; cancel != nil {
		cancel()
	}
	delete(manager.tokens, id)
	manager.mu.Unlock()
	manager.publish(job)
	manager.logRuntime("download cancelled job=%s repository=%q", job.ID, job.Repository)
	return manager.currentJob(id)
}

func (manager *Manager) Subscribe(id string) (<-chan DownloadJob, func()) {
	channel := make(chan DownloadJob, 8)
	manager.mu.Lock()
	if manager.subscribers[id] == nil {
		manager.subscribers[id] = map[chan DownloadJob]struct{}{}
	}
	manager.subscribers[id][channel] = struct{}{}
	manager.mu.Unlock()
	if job, found, err := manager.store.Job(id); err == nil && found {
		channel <- job
	}
	return channel, func() {
		manager.mu.Lock()
		if subscribers := manager.subscribers[id]; subscribers != nil {
			delete(subscribers, channel)
			if len(subscribers) == 0 {
				delete(manager.subscribers, id)
			}
		}
		manager.mu.Unlock()
	}
}

func (manager *Manager) Rescan() ([]ArtifactRecord, error) {
	artifacts := []ArtifactRecord{}
	err := filepath.WalkDir(manager.config.Storage.Root, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasSuffix(entry.Name(), ".hash") {
			return nil
		}
		if hash, trusted, err := ReadTrustedHashSidecar(filePath); err != nil {
			return err
		} else if trusted {
			record, err := artifactFromFile(filePath, hash, "", "", "", "sidecar")
			if err != nil {
				return err
			}
			if err := manager.recordArtifact(record); err != nil {
				return err
			}
			artifacts = append(artifacts, record)
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if record, found, err := manager.store.Artifact(filePath); err != nil {
			return err
		} else if found && record.Size == info.Size() && record.ModifiedUnixNano == info.ModTime().UnixNano() {
			if err := manager.notifyArtifact(record); err != nil {
				return err
			}
			artifacts = append(artifacts, record)
			return nil
		}
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
		artifacts = append(artifacts, record)
		return nil
	})
	return artifacts, err
}

func (manager *Manager) run(initial DownloadJob) {
	select {
	case manager.semaphore <- struct{}{}:
	case <-manager.lifecycle.Done():
		return
	}
	defer func() { <-manager.semaphore }()
	context, cancel := context.WithCancel(manager.lifecycle)
	manager.mu.Lock()
	current, found, err := manager.store.Job(initial.ID)
	if err != nil || !found || current.State != JobQueued {
		manager.mu.Unlock()
		cancel()
		if err != nil {
			manager.failJob(initial.ID, fmt.Errorf("read queued job: %w", err))
		}
		return
	}
	manager.running[initial.ID] = cancel
	manager.mu.Unlock()
	defer manager.releaseFinishedJob(initial.ID)
	job, _, err := manager.store.Job(initial.ID)
	if err != nil {
		manager.failJob(initial.ID, fmt.Errorf("read queued job before starting: %w", err))
		return
	}
	job.State = JobRunning
	if err := manager.store.SaveJob(job); err != nil {
		manager.failJob(job.ID, fmt.Errorf("mark job running: %w", err))
		return
	}
	manager.publish(job)
	manager.logRuntime("download started job=%s repository=%q", job.ID, job.Repository)
	if err := manager.transfer(context, job); err != nil {
		if manager.lifecycle.Err() != nil {
			return
		}
		current, found, readErr := manager.store.Job(job.ID)
		if readErr == nil && found && (current.State == JobPaused || current.State == JobCancelled) {
			return
		}
		if readErr != nil || !found {
			manager.failJob(job.ID, fmt.Errorf("transfer failed and job state could not be re-read: %w", err))
			return
		}
		current.State, current.Error = JobFailed, redactSensitive(err.Error())
		for index := range current.Files {
			if current.Files[index].State == string(JobRunning) {
				current.Files[index].State, current.Files[index].Error = string(JobFailed), current.Error
			}
		}
		if saveErr := manager.store.SaveJob(current); saveErr != nil {
			manager.logRuntime("download failed job=%s repository=%q but the failure state could not be saved: %v", current.ID, current.Repository, saveErr)
		}
		manager.publish(current)
		manager.logRuntime("download failed job=%s repository=%q error=%q", current.ID, current.Repository, current.Error)
		return
	}
	completed, found, err := manager.store.Job(job.ID)
	if err != nil || !found {
		manager.failJob(job.ID, fmt.Errorf("read job after a successful transfer: %w", errors.Join(err, boolError(!found, "job not found"))))
		return
	}
	if completed.State != JobRunning {
		return
	}
	completed.State, completed.Error, completed.CompletedBytes = JobCompleted, "", completed.TotalBytes
	for index := range completed.Files {
		completed.Files[index].State, completed.Files[index].CompletedBytes, completed.Files[index].Error = string(JobCompleted), completed.Files[index].Size, ""
	}
	if err := manager.store.SaveJob(completed); err == nil {
		manager.publish(completed)
		manager.logRuntime("download completed job=%s repository=%q bytes=%d", completed.ID, completed.Repository, completed.CompletedBytes)
	}
}

func (manager *Manager) failJob(jobID string, cause error) {
	message := redactSensitive(cause.Error())
	job, found, err := manager.store.Job(jobID)
	if err != nil || !found {
		manager.logRuntime("download failed job=%s error=%q and no stored row could be updated (read error: %v)", jobID, message, err)
		return
	}
	job.State, job.Error = JobFailed, message
	for index := range job.Files {
		if job.Files[index].State == string(JobQueued) || job.Files[index].State == string(JobRunning) {
			job.Files[index].State, job.Files[index].Error = string(JobFailed), message
		}
	}
	if saveErr := manager.store.SaveJob(job); saveErr != nil {
		manager.logRuntime("download failed job=%s repository=%q error=%q and the failure state could not be saved: %v", job.ID, job.Repository, message, saveErr)
		return
	}
	manager.publish(job)
	manager.logRuntime("download failed job=%s repository=%q error=%q", job.ID, job.Repository, message)
}

func boolError(condition bool, message string) error {
	if condition {
		return fmt.Errorf("%s", message)
	}
	return nil
}

func (manager *Manager) startJob(job DownloadJob) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.closed {
		return fmt.Errorf("downloader manager is closed")
	}
	manager.jobs.Add(1)
	go func() {
		defer manager.jobs.Done()
		manager.run(job)
	}()
	return nil
}

func newManagerLogger(config LoggingConfig) (*log.Logger, *os.File, error) {
	if config.Mode == "off" || strings.TrimSpace(config.Path) == "" {
		return log.New(io.Discard, "", 0), nil, nil
	}
	if err := ensureDirectory(filepath.Dir(config.Path)); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(config.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, err
	}
	return log.New(file, "", log.LstdFlags), file, nil
}

func (manager *Manager) logStartup(format string, values ...any) {
	manager.logger.Printf(format, values...)
}

func (manager *Manager) logRuntime(format string, values ...any) {
	if manager.config.Logging.Mode == "normal" {
		manager.logger.Printf(format, values...)
	}
}

func closeManagerLog(file *os.File) error {
	if file == nil {
		return nil
	}
	return file.Close()
}

func (manager *Manager) releaseFinishedJob(id string) {
	job, found, _ := manager.store.Job(id)
	manager.mu.Lock()
	defer manager.mu.Unlock()
	delete(manager.running, id)
	if found && (job.State == JobCompleted || job.State == JobFailed || job.State == JobCancelled) {
		delete(manager.tokens, id)
	}
}

func (manager *Manager) transfer(ctx context.Context, job DownloadJob) error {
	staging, err := manager.stagingDirectory(job)
	if err != nil {
		return err
	}
	completed := false
	defer func() {
		if completed {
			_ = os.RemoveAll(staging)
		}
	}()
	for _, file := range job.Files {
		current, found, err := manager.store.Job(job.ID)
		if err != nil || !found {
			return fmt.Errorf("download job disappeared")
		}
		if current.State != JobRunning {
			return context.Canceled
		}
		if file.State == string(JobCompleted) {
			continue
		}
		if err := manager.ensureFreeSpace(file.Size); err != nil {
			return err
		}
		if err := manager.setFileState(&current, file.Path, string(JobRunning), "", 0); err != nil {
			return err
		}
		if err := manager.store.SaveJob(current); err != nil {
			return err
		}
		manager.publish(current)
		stagedPath, err := secureStagingPath(staging, file.Path)
		if err != nil {
			return err
		}
		if err := manager.downloadFile(ctx, job.Repository, job.Commit, file.Path, stagedPath, file.Size, manager.jobToken(job.ID)); err != nil {
			return err
		}
		hash, size, err := SHA256File(stagedPath)
		if err != nil {
			return err
		}
		if file.Size > 0 && size != file.Size {
			return fmt.Errorf("downloaded size for %q differs from the planned size", file.Path)
		}
		if file.Size <= 0 && size == 0 {
			return fmt.Errorf("downloaded file %q is empty and Hugging Face reported no expected size", file.Path)
		}
		if expected := strings.TrimPrefix(file.ExpectedSHA256, "sha256:"); expected != "" && validSHA256(expected) && hash != expected {
			_ = os.Remove(stagedPath)
			return fmt.Errorf("downloaded hash for %q does not match the remote LFS SHA-256", file.Path)
		}
		if err := manager.promote(job, file, stagedPath, hash); err != nil {
			return err
		}
		current, found, err = manager.store.Job(job.ID)
		if err != nil || !found {
			return fmt.Errorf("download job disappeared")
		}
		if err := manager.setFileState(&current, file.Path, string(JobCompleted), "", size); err != nil {
			return err
		}
		current.CompletedBytes += size
		if err := manager.store.SaveJob(current); err != nil {
			return err
		}
		manager.publish(current)
	}
	if job.Snapshot {
		treeDigest, err := computeDownloadedSnapshotDigest(manager.config.Storage.Root, job.Repository, job.Commit, job.Files)
		if err != nil {
			return err
		}
		current, found, err := manager.store.Job(job.ID)
		if err != nil || !found {
			return fmt.Errorf("download job disappeared")
		}
		current.TreeSHA256 = treeDigest
		if err := manager.store.SaveJob(current); err != nil {
			return err
		}
	}
	completed = true
	return nil
}

func (manager *Manager) promote(job DownloadJob, file JobFile, stagedPath string, hash string) error {
	destination, err := downloadDestinationPath(manager.config.Storage.Root, job.Repository, job.Commit, job.Snapshot, file.Path)
	if err != nil {
		return err
	}
	if err := ensureDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	if job.Snapshot {
		info, statErr := os.Lstat(destination)
		if statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return fmt.Errorf("immutable snapshot destination is not a regular file")
			}
			existingHash, existingSize, hashErr := SHA256File(destination)
			if hashErr != nil {
				return hashErr
			}
			if existingHash != hash || existingSize != file.Size {
				return fmt.Errorf("immutable snapshot destination already exists with different content")
			}
			return nil
		}
		if !os.IsNotExist(statErr) {
			return statErr
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
		backup = destination + ".tensor-router-replaced-" + job.ID
		if err := os.Rename(destination, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(temporary, destination); err != nil {
		if backup != "" {
			_ = os.Rename(backup, destination)
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
	if err := manager.recordArtifact(record); err != nil {
		return err
	}
	if manager.config.Scanning.WriteHashSidecars && !job.Snapshot {
		return WriteHashSidecar(destination, hash)
	}
	return nil
}

func (manager *Manager) recordArtifact(record ArtifactRecord) error {
	if err := manager.store.SaveArtifact(record); err != nil {
		return err
	}
	return manager.notifyArtifact(record)
}

func (manager *Manager) notifyArtifact(record ArtifactRecord) error {
	manager.mu.Lock()
	handler := manager.artifactHandler
	manager.mu.Unlock()
	if handler == nil {
		return nil
	}
	return handler(record)
}

func (manager *Manager) stagingDirectory(job DownloadJob) (string, error) {
	return secureJoin(manager.config.Storage.StateDir, "staging", job.ID)
}

func copyPromotionFile(stagedPath string, destination string, expectedHash string) (string, error) {
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
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".tensor-router-promote-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
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

func preparePromotionFile(jobID string, stagedPath string, destination string, expectedHash string) (string, error) {
	if !safeRepositoryPart(jobID) {
		return "", fmt.Errorf("download job ID is invalid")
	}
	temporaryPath := filepath.Join(filepath.Dir(destination), "."+filepath.Base(destination)+".tensor-router-promote-"+jobID)
	if _, err := os.Lstat(temporaryPath); err == nil {
		return "", fmt.Errorf("promotion path already exists")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(stagedPath, temporaryPath); err == nil {
		return temporaryPath, nil
	}
	return copyPromotionFile(stagedPath, destination, expectedHash)
}

func secureStagingPath(staging string, repositoryPath string) (string, error) {
	parts, err := safeRepositoryPath(repositoryPath)
	if err != nil {
		return "", err
	}
	return secureJoin(staging, parts...)
}

func (manager *Manager) ensureReplacementAllowed(plan DownloadPlan, confirmed bool) error {
	for _, file := range plan.Files {
		destination, err := downloadDestinationResolve(manager.config.Storage.Root, plan.Repository, plan.Commit, plan.Snapshot, file.Path)
		if err != nil {
			return err
		}
		record, found, err := manager.store.Artifact(destination)
		if err != nil {
			return err
		}
		if found && record.Revision == plan.Commit {
			continue
		}
		if _, err := os.Lstat(destination); err == nil && plan.Snapshot {
			return fmt.Errorf("immutable snapshot destination already exists")
		} else if err == nil && !confirmed {
			return fmt.Errorf("repository revision replacement requires explicit confirmation")
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (manager *Manager) ensureFreeSpace(required int64) error {
	available, known, err := availableSpace(manager.config.Storage.Root)
	if err != nil {
		return err
	}
	if !known {
		return nil
	}
	reserve := manager.config.Storage.FreeSpaceReserveGB << 30
	if available-required < reserve {
		return fmt.Errorf("insufficient storage space after preserving the configured reserve")
	}
	return nil
}

func (manager *Manager) setFileState(job *DownloadJob, path string, state string, problem string, completed int64) error {
	for index := range job.Files {
		if job.Files[index].Path == path {
			job.Files[index].State, job.Files[index].Error, job.Files[index].CompletedBytes = state, problem, completed
			return nil
		}
	}
	return fmt.Errorf("download job file was not found")
}

func (manager *Manager) token(operationToken string) string {
	if token := strings.TrimSpace(operationToken); token != "" {
		return token
	}
	return strings.TrimSpace(manager.config.HuggingFace.Token)
}

func (manager *Manager) jobToken(id string) string {
	manager.mu.Lock()
	token := manager.tokens[id]
	manager.mu.Unlock()
	return manager.token(token)
}

func (manager *Manager) currentJob(id string) (DownloadJob, error) {
	job, found, err := manager.store.Job(id)
	if err != nil {
		return DownloadJob{}, err
	}
	if !found {
		return DownloadJob{}, fmt.Errorf("download job was not found")
	}
	return job, nil
}

func (manager *Manager) publish(job DownloadJob) {
	manager.mu.Lock()
	channels := make([]chan DownloadJob, 0, len(manager.subscribers[job.ID]))
	for channel := range manager.subscribers[job.ID] {
		channels = append(channels, channel)
	}
	manager.mu.Unlock()
	for _, channel := range channels {
		if job.State.Terminal() {
			timer := time.NewTimer(2 * time.Second)
			select {
			case channel <- job:
			case <-timer.C:
			}
			timer.Stop()
			continue
		}
		select {
		case channel <- job:
		default:
		}
	}
}

func redactSensitive(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret = strings.TrimSpace(secret); secret != "" {
			value = strings.ReplaceAll(value, secret, "[redacted]")
		}
	}
	fields := strings.Fields(value)
	for index, field := range fields {
		lower := strings.ToLower(field)
		if strings.Contains(lower, "token") || strings.HasPrefix(field, "hf_") || strings.HasPrefix(field, "sk-") {
			fields[index] = "[redacted]"
		}
	}
	return strings.Join(fields, " ")
}

func randomJobID() string {
	buffer := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
