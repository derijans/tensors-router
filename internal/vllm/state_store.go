package vllm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (manager *Manager) loadPersistentState() error {
	manager.launchOptions = DefaultLaunchOptions()
	if err := readJSONIfExists(manager.launchOptionsPath(), &manager.launchOptions); err != nil {
		return fmt.Errorf("load vLLM launch options: %w", err)
	}
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

func (manager *Manager) launchOptionsPath() string {
	return filepath.Join(manager.dataDir, "state", "launch-options.json")
}

// LaunchOptions reports the persisted launch options, falling back to the fully offline
// defaults when an operator has never chosen anything.
func (manager *Manager) LaunchOptions(context.Context) (LaunchOptions, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.launchOptions, nil
}

// SetLaunchOptions persists the selection and unloads every running runtime so the next
// load starts with the new environment. Persisting before unloading means a crash
// between the two still leaves the stored choice authoritative.
func (manager *Manager) SetLaunchOptions(ctx context.Context, options LaunchOptions) (LaunchOptions, error) {
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return LaunchOptions{}, fmt.Errorf("vLLM manager is closed")
	}
	previous := manager.launchOptions
	manager.launchOptions = options
	if err := writeJSONAtomic(manager.launchOptionsPath(), options, 0o600); err != nil {
		manager.launchOptions = previous
		manager.mu.Unlock()
		return LaunchOptions{}, fmt.Errorf("persist vLLM launch options: %w", err)
	}
	kinds := make([]RuntimeKind, 0, len(manager.runtimes))
	for kind := range manager.runtimes {
		kinds = append(kinds, kind)
	}
	manager.mu.Unlock()
	var unloadError error
	for _, kind := range kinds {
		unloadError = errors.Join(unloadError, manager.Unload(ctx, kind))
	}
	if unloadError != nil {
		return options, fmt.Errorf("apply vLLM launch options: %w", unloadError)
	}
	return options, nil
}

func (manager *Manager) currentLaunchOptions() LaunchOptions {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return manager.launchOptions
}
