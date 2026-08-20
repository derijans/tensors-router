package vllm

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
)

func (manager *Manager) StartInitialization(_ context.Context, request InitRequest) (InitializationJob, error) {
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return InitializationJob{}, fmt.Errorf("vLLM manager is closed")
	}
	if manager.job.State == JobQueued || manager.job.State == JobRunning {
		job := manager.job
		manager.mu.Unlock()
		return job, nil
	}
	manager.mu.Unlock()
	requestedProfile := strings.TrimSpace(request.Profile)
	if requestedProfile == "" {
		requestedProfile = manager.options.DefaultProfile
	}
	now := manager.now().UTC()
	job := InitializationJob{
		JobID:           uuid.NewString(),
		BackendID:       BackendID,
		State:           JobQueued,
		SelectedProfile: requestedProfile,
		Phase:           "authorizing_manifest",
		Retryable:       true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	manager.mu.Lock()
	if manager.job.State == JobQueued || manager.job.State == JobRunning {
		existing := manager.job
		manager.mu.Unlock()
		return existing, nil
	}
	manager.job = job
	if err := manager.saveJobLocked(); err != nil {
		manager.job = InitializationJob{}
		manager.mu.Unlock()
		return InitializationJob{}, err
	}
	jobContext, cancel := context.WithCancel(context.Background())
	manager.worker = &initializationWorker{jobID: job.JobID, cancel: cancel}
	manager.workers.Add(1)
	manager.mu.Unlock()
	go func() {
		defer manager.workers.Done()
		manager.prepareInitialization(jobContext, job.JobID, requestedProfile)
	}()
	return job, nil
}

func (manager *Manager) prepareInitialization(ctx context.Context, jobID string, requestedProfile string) {
	if err := manager.updateJob(jobID, "authorizing_manifest", 0); err != nil {
		return
	}
	manifest, manifestDigest, manifestTrust, err := ResolveManifest(ctx, manager.options.ManifestSource)
	if err != nil {
		manager.finishJobFromContext(jobID, ctx, err)
		return
	}
	if manifestTrust != ManifestTrustTUF {
		log.Printf("vLLM manifest is not TUF-authorized: trust tier %s; artifacts remain pinned by the manifest digest %s", manifestTrust, manifestDigest)
	}
	if err := manager.updateJob(jobID, "detecting_prerequisites", 0); err != nil {
		return
	}
	detection, err := manager.options.Detector.Detect(ctx)
	if err != nil {
		manager.finishJobFromContext(jobID, ctx, fmt.Errorf("detect vLLM prerequisites: %w", err))
		return
	}
	profile, err := SelectProfile(manifest, requestedProfile, detection)
	if err != nil {
		manager.finishJobFromContext(jobID, ctx, err)
		return
	}
	totalBytes, err := profileArtifactBytes(profile)
	if err != nil {
		manager.finishJobFromContext(jobID, ctx, err)
		return
	}
	manager.mu.Lock()
	if manager.job.JobID != jobID || manager.job.State == JobCancelled {
		manager.mu.Unlock()
		return
	}
	manager.job.SelectedProfile = profile.ID
	manager.job.DetectedProfile = strings.Join(detection.Devices, ",")
	manager.job.ManifestSHA256 = manifestDigest
	manager.job.ManifestTrust = string(manifestTrust)
	manager.job.TotalBytes = totalBytes
	manager.job.UpdatedAt = manager.now().UTC()
	if err := manager.saveJobLocked(); err != nil {
		manager.failJobPersistenceLocked(jobID, err)
		manager.mu.Unlock()
		return
	}
	manager.mu.Unlock()
	manager.executeInitialization(ctx, jobID, profile, manifestDigest)
}

func (manager *Manager) CancelInitialization(context.Context) (InitializationJob, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.job.JobID == "" {
		return InitializationJob{}, fmt.Errorf("vLLM initialization job does not exist")
	}
	if manager.job.State != JobQueued && manager.job.State != JobRunning {
		return manager.job, nil
	}
	previous := manager.job
	manager.job.State = JobCancelled
	manager.job.Phase = "cancelled"
	manager.job.Error = ""
	manager.job.Retryable = true
	manager.job.UpdatedAt = manager.now().UTC()
	if err := manager.saveJobLocked(); err != nil {
		manager.job = previous
		return InitializationJob{}, err
	}
	manager.clearWorkerLocked(manager.job.JobID, true)
	return manager.job, nil
}

func (manager *Manager) executeInitialization(ctx context.Context, jobID string, profile Profile, manifestDigest string) {
	environmentID, err := environmentID(profile, manifestDigest)
	if err != nil {
		manager.finishJob(jobID, JobFailed, "failed", err)
		return
	}
	stagePath := filepath.Join(manager.dataDir, "staging", jobID)
	if err := ensurePrivateDirectory(stagePath); err != nil {
		manager.finishJob(jobID, JobFailed, "failed", err)
		return
	}
	defer removeOwnedDirectory(filepath.Join(manager.dataDir, "staging"), stagePath)

	if err := manager.updateJob(jobID, "downloading", 0); err != nil {
		return
	}
	artifacts, err := manager.downloadArtifacts(ctx, jobID, profile, stagePath)
	if err != nil {
		manager.finishJobFromContext(jobID, ctx, err)
		return
	}
	environmentStage := filepath.Join(stagePath, "environment")
	if err := ensurePrivateDirectory(environmentStage); err != nil {
		manager.finishJob(jobID, JobFailed, "failed", err)
		return
	}
	if err := manager.updateJob(jobID, "installing", -1); err != nil {
		return
	}
	if err := manager.options.Installer.Install(ctx, profile, artifacts, environmentStage, func(phase string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return manager.updateJob(jobID, sanitizePhase(phase), -1)
	}); err != nil {
		manager.finishJobFromContext(jobID, ctx, err)
		return
	}
	if err := manager.updateJob(jobID, "smoke_testing", -1); err != nil {
		return
	}
	if err := manager.options.SmokeTester.Test(ctx, profile, environmentStage); err != nil {
		manager.finishJobFromContext(jobID, ctx, fmt.Errorf("vLLM smoke test failed: %w", err))
		return
	}
	if err := ctx.Err(); err != nil {
		manager.finishJobFromContext(jobID, ctx, err)
		return
	}
	containerEngine, err := installedContainerEngine(profile, environmentStage)
	if err != nil {
		manager.finishJob(jobID, JobFailed, "failed", err)
		return
	}
	marker := environmentMarker{ProfileID: profile.ID, VLLMVersion: profile.VLLMVersion, ManifestSHA256: manifestDigest, CreatedAt: manager.now().UTC(), InstallMethod: profile.InstallMethod, OCIImage: profile.OCIImage, ContainerEngine: containerEngine, Devices: append([]string{}, profile.Devices...)}
	if err := writeJSONAtomic(filepath.Join(environmentStage, "environment.json"), marker, 0o600); err != nil {
		manager.finishJob(jobID, JobFailed, "failed", err)
		return
	}
	if err := manager.updateJob(jobID, "promoting", -1); err != nil {
		return
	}
	finalPath := filepath.Join(manager.dataDir, "environments", environmentID)
	if err := promoteEnvironment(environmentStage, finalPath); err != nil {
		manager.finishJob(jobID, JobFailed, "failed", err)
		return
	}
	active := activeEnvironment{ProfileID: profile.ID, VLLMVersion: profile.VLLMVersion, ManifestSHA256: manifestDigest, Path: finalPath, InstallMethod: profile.InstallMethod, OCIImage: profile.OCIImage, ContainerEngine: containerEngine, Devices: append([]string{}, profile.Devices...)}
	if err := validateActiveEnvironment(manager.dataDir, active); err != nil {
		manager.finishJob(jobID, JobFailed, "failed", fmt.Errorf("validate promoted vLLM environment: %w", err))
		return
	}
	manager.mu.Lock()
	if manager.job.JobID != jobID || manager.job.State == JobCancelled || ctx.Err() != nil {
		manager.mu.Unlock()
		return
	}
	if err := writeJSONAtomic(manager.activePath(), active, 0o600); err != nil {
		manager.job.State = JobFailed
		manager.job.Phase = "failed"
		manager.job.Error = errorText(sanitizeError(err))
		manager.job.Retryable = true
		manager.job.UpdatedAt = manager.now().UTC()
		if saveError := manager.saveJobLocked(); saveError != nil {
			manager.failJobPersistenceLocked(jobID, saveError)
		} else {
			manager.clearWorkerLocked(jobID, false)
		}
		manager.mu.Unlock()
		return
	}
	manager.active = active
	manager.job.State = JobCompleted
	manager.job.Phase = "completed"
	manager.job.CompletedBytes = manager.job.TotalBytes
	manager.job.Error = ""
	manager.job.Retryable = false
	manager.job.UpdatedAt = manager.now().UTC()
	if err := manager.saveJobLocked(); err != nil {
		manager.job.Error = errorText(sanitizeError(fmt.Errorf("persist completed vLLM initialization job: %w", err)))
	}
	manager.clearWorkerLocked(jobID, false)
	manager.mu.Unlock()
}

func (manager *Manager) downloadArtifacts(ctx context.Context, jobID string, profile Profile, stagePath string) (map[string]string, error) {
	downloadPath := filepath.Join(stagePath, "artifacts")
	if err := ensurePrivateDirectory(downloadPath); err != nil {
		return nil, err
	}
	paths := make(map[string]string, len(profile.Artifacts))
	var completedBefore int64
	for _, artifact := range profile.Artifacts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		destination := filepath.Join(downloadPath, artifact.Name)
		var progressError error
		var progressMutex sync.Mutex
		if err := manager.options.Downloader.Download(ctx, artifact, destination, func(current int64) {
			if current < 0 {
				current = 0
			}
			if current > artifact.Size {
				current = artifact.Size
			}
			progressMutex.Lock()
			if progressError == nil {
				progressError = manager.updateJob(jobID, "downloading", completedBefore+current)
			}
			progressMutex.Unlock()
		}); err != nil {
			progressMutex.Lock()
			persistError := progressError
			progressMutex.Unlock()
			if persistError != nil {
				return nil, persistError
			}
			return nil, fmt.Errorf("download artifact %q: %w", artifact.Name, err)
		}
		progressMutex.Lock()
		persistError := progressError
		progressMutex.Unlock()
		if persistError != nil {
			return nil, persistError
		}
		if err := VerifyArtifactFile(destination, artifact); err != nil {
			return nil, err
		}
		completedBefore += artifact.Size
		if err := manager.updateJob(jobID, "downloading", completedBefore); err != nil {
			return nil, err
		}
		paths[artifact.Name] = destination
	}
	return paths, nil
}

func (manager *Manager) updateJob(jobID string, phase string, completedBytes int64) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.job.JobID != jobID || manager.job.State == JobCancelled {
		return context.Canceled
	}
	manager.job.State = JobRunning
	manager.job.Phase = phase
	if completedBytes >= 0 {
		manager.job.CompletedBytes = completedBytes
	}
	manager.job.UpdatedAt = manager.now().UTC()
	if err := manager.saveJobLocked(); err != nil {
		manager.failJobPersistenceLocked(jobID, err)
		return fmt.Errorf("persist vLLM initialization progress: %w", err)
	}
	return nil
}

func (manager *Manager) finishJobFromContext(jobID string, ctx context.Context, err error) {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		manager.finishJob(jobID, JobCancelled, "cancelled", nil)
		return
	}
	manager.finishJob(jobID, JobFailed, "failed", err)
}

func (manager *Manager) finishJob(jobID string, state string, phase string, err error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.job.JobID != jobID || manager.job.State == JobCancelled {
		return
	}
	manager.job.State = state
	manager.job.Phase = phase
	manager.job.Error = errorText(sanitizeError(err))
	manager.job.Retryable = state != JobCompleted
	manager.job.UpdatedAt = manager.now().UTC()
	if saveError := manager.saveJobLocked(); saveError != nil {
		manager.failJobPersistenceLocked(jobID, saveError)
		return
	}
	manager.clearWorkerLocked(jobID, false)
}

func (manager *Manager) resumeInitialization() {
	job := manager.job
	ctx, cancel := context.WithCancel(context.Background())
	manager.worker = &initializationWorker{jobID: job.JobID, cancel: cancel}
	if err := clearInitializationStaging(manager.dataDir, job.JobID); err != nil {
		manager.finishJob(job.JobID, JobFailed, "recovery_failed", fmt.Errorf("clear stale vLLM initialization staging: %w", err))
		return
	}
	manager.workers.Add(1)
	go func() {
		defer manager.workers.Done()
		manager.prepareInitialization(ctx, job.JobID, job.SelectedProfile)
	}()
}

func (manager *Manager) failJobPersistenceLocked(jobID string, err error) {
	if manager.job.JobID != jobID {
		return
	}
	manager.job.State = JobFailed
	manager.job.Phase = "persistence_failed"
	manager.job.Error = errorText(sanitizeError(fmt.Errorf("persist vLLM initialization state: %w", err)))
	manager.job.Retryable = true
	manager.job.UpdatedAt = manager.now().UTC()
	manager.clearWorkerLocked(jobID, true)
}

func (manager *Manager) clearWorkerLocked(jobID string, cancel bool) {
	if manager.worker == nil || manager.worker.jobID != jobID {
		return
	}
	if cancel {
		manager.worker.cancel()
	}
	manager.worker = nil
}
