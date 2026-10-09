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
	retryWait       retryWaiter
	lifecycle       context.Context
	stopLifecycle   context.CancelFunc
	mu              sync.Mutex
	running         map[string]*jobRun
	tokens          map[string]string
	subscribers     map[string]map[chan DownloadJob]struct{}
	reservedBytes   int64
	semaphore       chan struct{}
	artifactHandler ArtifactHandler
	library         unhashedLibrary
	scanMu          sync.Mutex
	scans           sync.WaitGroup
	logger          *log.Logger
	logFile         *os.File
	jobs            sync.WaitGroup
	closed          bool
	closeOnce       sync.Once
	closeError      error
}

type ArtifactHandler func(ArtifactRecord) error

func NewManager(config Config, _ string) (*Manager, error) {
	storage, err := resolveStorageLocations(config.Storage)
	if err != nil {
		return nil, err
	}
	config.Storage = storage
	logger, logFile, err := newManagerLogger(config.Logging)
	if err != nil {
		return nil, fmt.Errorf("initialize downloader logging: %w", err)
	}
	store, err := OpenStore(config.Storage.DatabasePath)
	if err != nil {
		_ = closeManagerLog(logFile)
		return nil, fmt.Errorf("initialize downloader database: %w", err)
	}
	lifecycle, stopLifecycle := context.WithCancel(context.Background())
	manager := &Manager{
		config:        config,
		store:         store,
		hub:           NewHubClient(config.HuggingFace.Endpoint, config.Downloads.Timeout),
		retryWait:     waitWithContext,
		lifecycle:     lifecycle,
		stopLifecycle: stopLifecycle,
		running:       map[string]*jobRun{},
		tokens:        map[string]string{},
		subscribers:   map[string]map[chan DownloadJob]struct{}{},
		semaphore:     make(chan struct{}, config.Downloads.ConcurrentJobs),
		logger:        logger,
		logFile:       logFile,
	}
	manager.logStartup("downloader initialized storage=%q endpoint=%q", config.Storage.Root, config.HuggingFace.Endpoint)
	return manager, nil
}

func (manager *Manager) Close() error {
	manager.closeOnce.Do(func() {
		manager.mu.Lock()
		manager.closed = true
		manager.stopLifecycle()
		manager.mu.Unlock()
		manager.jobs.Wait()
		manager.scans.Wait()
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
	plan, err := BuildPlan(details, request.Files, request.Mode, manager.config.Storage.Root)
	if err != nil {
		return DownloadPlan{}, err
	}
	return manager.withoutPresentFiles(plan)
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
	if plan.AlreadyPresent() {
		return DownloadJob{}, fmt.Errorf("every planned file is already present with the same hash")
	}
	if len(plan.Files) == 0 {
		return DownloadJob{}, fmt.Errorf("download plan resolved to zero files; the repository, revision, or file selection matched nothing on Hugging Face")
	}
	if plan.UnsafeWarning && !confirmUnsafe {
		return DownloadJob{}, fmt.Errorf("repository has an unsafe or pending security status; explicit confirmation is required")
	}
	if existing, found, err := manager.store.ActiveJobFor(plan.Repository, plan.Commit, plan.Snapshot, plannedPaths(plan.Files)); err != nil {
		return DownloadJob{}, err
	} else if found {
		manager.rememberToken(existing.ID, operationToken)
		manager.logRuntime("download request joined active job=%s repository=%q", existing.ID, existing.Repository)
		return existing, nil
	}
	if err := manager.checkFreeSpace(plan.TotalBytes); err != nil {
		return DownloadJob{}, err
	}
	if err := manager.ensureReplacementAllowed(plan, confirmReplace); err != nil {
		return DownloadJob{}, err
	}
	job := DownloadJob{ID: randomJobID(), Repository: plan.Repository, Revision: plan.Revision, Commit: plan.Commit, State: JobQueued, TotalBytes: plan.TotalBytes, Snapshot: plan.Snapshot, Files: make([]JobFile, 0, len(plan.Files))}
	for _, file := range plan.Files {
		job.Files = append(job.Files, JobFile{Path: file.Path, Reason: file.Reason, ExpectedSHA256: file.LFSHash, ExpectedGitOID: file.GitOID, Size: file.Size, State: string(JobQueued)})
	}
	if err := manager.store.SaveJob(job); err != nil {
		return DownloadJob{}, err
	}
	manager.rememberToken(job.ID, operationToken)
	stored, err := manager.currentJob(job.ID)
	if err != nil {
		return DownloadJob{}, err
	}
	manager.logRuntime("download queued job=%s repository=%q files=%d bytes=%d", stored.ID, stored.Repository, len(stored.Files), stored.TotalBytes)
	if err := manager.startJob(stored.ID); err != nil {
		return DownloadJob{}, err
	}
	return stored, nil
}

func (manager *Manager) Job(id string) (DownloadJob, bool, error) { return manager.store.Job(id) }

func (manager *Manager) Jobs() ([]DownloadJob, error) { return manager.store.Jobs() }

func (manager *Manager) Artifacts() ([]ArtifactRecord, error) { return manager.store.ListArtifacts() }

func (manager *Manager) Pause(id string) (DownloadJob, error) {
	if err := manager.transition(id, "paused", JobPaused, JobQueued, JobRunning); err != nil {
		return DownloadJob{}, err
	}
	manager.cancelRun(id)
	manager.publishCurrent(id)
	manager.logRuntime("download paused job=%s", id)
	return manager.currentJob(id)
}

func (manager *Manager) Resume(id string) (DownloadJob, error) {
	if err := manager.transition(id, "resumed", JobQueued, JobPaused, JobFailed); err != nil {
		return DownloadJob{}, err
	}
	if err := manager.store.ResetUnfinishedFiles(id); err != nil {
		return DownloadJob{}, err
	}
	manager.logRuntime("download resumed job=%s", id)
	if err := manager.startJob(id); err != nil {
		return DownloadJob{}, err
	}
	return manager.currentJob(id)
}

func (manager *Manager) Cancel(id string) (DownloadJob, error) {
	if err := manager.transition(id, "cancelled", JobCancelled, JobQueued, JobRunning, JobPaused, JobFailed); err != nil {
		return DownloadJob{}, err
	}
	manager.mu.Lock()
	run := manager.running[id]
	delete(manager.tokens, id)
	manager.mu.Unlock()
	if run != nil {
		run.cancel()
	} else {
		manager.removeStaging(id)
	}
	manager.publishCurrent(id)
	manager.logRuntime("download cancelled job=%s", id)
	return manager.currentJob(id)
}

func (manager *Manager) transition(id string, verb string, to JobState, from ...JobState) error {
	job, err := manager.currentJob(id)
	if err != nil {
		return err
	}
	changed, err := manager.store.TransitionJob(id, to, "", from...)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("download job cannot be %s from %s", verb, job.State)
	}
	return nil
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

func (manager *Manager) recordArtifact(record ArtifactRecord) (ArtifactRecord, error) {
	saved, err := manager.store.SaveArtifact(record)
	if err != nil {
		return ArtifactRecord{}, err
	}
	manager.library.forget(saved.Path)
	return saved, manager.notifyArtifact(saved)
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

func (manager *Manager) stagingDirectory(jobID string) (string, error) {
	return secureJoin(manager.config.Storage.StateDir, "staging", jobID)
}

func (manager *Manager) removeStaging(jobID string) {
	if !safeRepositoryPart(jobID) {
		return
	}
	staging, err := secureResolve(manager.config.Storage.StateDir, "staging", jobID)
	if err != nil {
		manager.logRuntime("download staging cleanup refused job=%s error=%q", jobID, err)
		return
	}
	if err := os.RemoveAll(staging); err != nil {
		manager.logRuntime("download staging cleanup failed job=%s error=%q", jobID, err)
	}
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

func (manager *Manager) rememberToken(jobID string, operationToken string) {
	token := strings.TrimSpace(operationToken)
	if token == "" {
		return
	}
	manager.mu.Lock()
	manager.tokens[jobID] = token
	manager.mu.Unlock()
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

func (manager *Manager) publishCurrent(id string) {
	if job, found, err := manager.store.Job(id); err == nil && found {
		manager.publish(job)
	}
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

func newManagerLogger(config LoggingConfig) (*log.Logger, *os.File, error) {
	if config.Mode == "off" || strings.TrimSpace(config.Path) == "" {
		return log.New(io.Discard, "", 0), nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(config.Path), 0o700); err != nil {
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

func randomJobID() string {
	buffer := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buffer)
}
