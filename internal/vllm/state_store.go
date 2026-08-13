package vllm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (manager *Manager) loadPersistentState() error {
	if err := readJSONIfExists(manager.jobPath(), &manager.job); err != nil {
		return fmt.Errorf("load vLLM initialization job: %w", err)
	}
	if err := validatePersistentJob(manager.job); err != nil {
		return fmt.Errorf("load vLLM initialization job: %w", err)
	}
	if err := readJSONIfExists(manager.activePath(), &manager.active); err != nil {
		return fmt.Errorf("load active vLLM environment: %w", err)
	}
	if manager.active.Path == "" {
		return nil
	}
	if err := validateActiveEnvironment(manager.dataDir, manager.active); err != nil {
		manager.active = activeEnvironment{}
		return nil
	}
	if manager.job.State == JobQueued || manager.job.State == JobRunning {
		if manager.job.ManifestSHA256 != "" && equalSHA256(manager.job.ManifestSHA256, manager.active.ManifestSHA256) && manager.job.SelectedProfile == manager.active.ProfileID {
			manager.job.State = JobCompleted
			manager.job.Phase = "completed"
			manager.job.CompletedBytes = manager.job.TotalBytes
			manager.job.Error = ""
			manager.job.Retryable = false
			manager.job.UpdatedAt = manager.now().UTC()
			if err := manager.saveJobLocked(); err != nil {
				return fmt.Errorf("reconcile completed vLLM initialization job: %w", err)
			}
		}
	}
	return nil
}

func validatePersistentJob(job InitializationJob) error {
	if job.JobID == "" {
		if job.BackendID == "" && job.State == "" {
			return nil
		}
		return fmt.Errorf("job id is required")
	}
	if !safeIdentifier(job.JobID) || job.BackendID != BackendID {
		return fmt.Errorf("job identity is invalid")
	}
	switch job.State {
	case JobQueued, JobRunning, JobCompleted, JobFailed, JobCancelled:
	default:
		return fmt.Errorf("job state %q is invalid", job.State)
	}
	if job.SelectedProfile != "auto" && !safeIdentifier(job.SelectedProfile) {
		return fmt.Errorf("selected profile is invalid")
	}
	if job.ManifestSHA256 != "" && !validSHA256(job.ManifestSHA256) {
		return fmt.Errorf("manifest digest is invalid")
	}
	if job.Phase != "" && !safeIdentifier(job.Phase) {
		return fmt.Errorf("job phase is invalid")
	}
	if strings.ContainsAny(job.DetectedProfile, "\x00\r\n") {
		return fmt.Errorf("detected profile is invalid")
	}
	if job.CompletedBytes < 0 || job.TotalBytes < 0 || job.TotalBytes > 0 && job.CompletedBytes > job.TotalBytes {
		return fmt.Errorf("job progress is invalid")
	}
	if job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() || job.UpdatedAt.Before(job.CreatedAt) {
		return fmt.Errorf("job timestamps are invalid")
	}
	return nil
}

func validateActiveEnvironment(dataDir string, active activeEnvironment) error {
	if !safeIdentifier(active.ProfileID) || validatePinnedVersion("vLLM version", active.VLLMVersion) != nil || !validSHA256(active.ManifestSHA256) {
		return fmt.Errorf("active vLLM environment identity is invalid")
	}
	switch active.InstallMethod {
	case "", "wheel", "source":
		if active.OCIImage != "" || active.ContainerEngine != "" {
			return fmt.Errorf("non-OCI environment contains OCI metadata")
		}
	case "oci":
		if !validOCIImage(active.OCIImage) || active.ContainerEngine != "docker" && active.ContainerEngine != "podman" {
			return fmt.Errorf("active OCI environment metadata is invalid")
		}
	default:
		return fmt.Errorf("active vLLM installation method is invalid")
	}
	for _, device := range active.Devices {
		if !validDevice(strings.ToLower(strings.TrimSpace(device))) {
			return fmt.Errorf("active vLLM device is invalid")
		}
	}
	environmentsRoot := filepath.Join(dataDir, "environments")
	if err := requirePathWithin(environmentsRoot, active.Path); err != nil {
		return err
	}
	var marker environmentMarker
	if err := readJSONRegular(filepath.Join(active.Path, "environment.json"), &marker, 1<<20); err != nil {
		return err
	}
	if marker.ProfileID != active.ProfileID || marker.VLLMVersion != active.VLLMVersion || !equalSHA256(marker.ManifestSHA256, active.ManifestSHA256) || marker.InstallMethod != active.InstallMethod || marker.OCIImage != active.OCIImage || marker.ContainerEngine != active.ContainerEngine || !equalStrings(marker.Devices, active.Devices) {
		return fmt.Errorf("active vLLM environment marker does not match state")
	}
	return nil
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (manager *Manager) saveJobLocked() error {
	return manager.jobWriter(manager.jobPath(), manager.job, 0o600)
}

func (manager *Manager) jobPath() string {
	return filepath.Join(manager.dataDir, "state", "initialization-job.json")
}

func (manager *Manager) activePath() string {
	return filepath.Join(manager.dataDir, "state", "active-environment.json")
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q must be a real directory", path)
	}
	return os.Chmod(path, 0o700)
}

func promoteEnvironment(stagePath string, finalPath string) error {
	if info, err := os.Lstat(finalPath); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("vLLM environment destination is unsafe")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stagePath, finalPath); err != nil {
		return fmt.Errorf("atomically promote vLLM environment: %w", err)
	}
	return syncDirectory(filepath.Dir(finalPath))
}

func writeJSONAtomic(path string, value any, mode os.FileMode) error {
	content, err := json.Marshal(value)
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	if err := ensurePrivateDirectory(directory); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".vllm-state-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	if err := temporary.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		_ = os.Remove(temporaryPath)
		return err
	}
	return syncDirectory(directory)
}

func readJSONIfExists(path string, target any) error {
	err := readJSONRegular(path, target, 1<<20)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func readJSONRegular(path string, target any, limit int64) error {
	content, err := readBoundedRegularFile(path, limit)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func requirePathWithin(rootPath string, candidatePath string) error {
	root, err := filepath.Abs(rootPath)
	if err != nil {
		return err
	}
	candidate, err := filepath.Abs(candidatePath)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes owned directory %q", candidate, root)
	}
	return nil
}

func removeOwnedDirectory(rootPath string, candidatePath string) {
	if requirePathWithin(rootPath, candidatePath) == nil {
		_ = os.RemoveAll(candidatePath)
	}
}

func clearInitializationStaging(dataDir string, jobID string) error {
	stagingRoot := filepath.Join(dataDir, "staging")
	stagePath := filepath.Join(stagingRoot, jobID)
	if err := requirePathWithin(stagingRoot, stagePath); err != nil {
		return err
	}
	return os.RemoveAll(stagePath)
}
