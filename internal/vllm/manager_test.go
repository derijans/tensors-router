package vllm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type staticManifestSource struct {
	manifest Manifest
	err      error
	calls    atomic.Int32
}

func (source *staticManifestSource) Load(context.Context) (Manifest, string, error) {
	source.calls.Add(1)
	if source.err != nil {
		return Manifest{}, "", source.err
	}
	return source.manifest, strings.Repeat("b", 64), nil
}

type staticDetector struct {
	detection Detection
	calls     atomic.Int32
}

func (detector *staticDetector) Detect(context.Context) (Detection, error) {
	detector.calls.Add(1)
	return detector.detection, nil
}

type writingDownloader struct {
	calls atomic.Int32
}

func (downloader *writingDownloader) Download(_ context.Context, artifact Artifact, destination string, progress func(int64)) error {
	downloader.calls.Add(1)
	content := []byte("uv")
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	progress(int64(len(content)))
	return nil
}

type controlledInstaller struct {
	mu      sync.Mutex
	block   <-chan struct{}
	started chan struct{}
	calls   atomic.Int32
}

func (installer *controlledInstaller) Install(ctx context.Context, _ Profile, _ map[string]string, environmentPath string, phase func(string) error) error {
	installer.calls.Add(1)
	if err := phase("installing_packages"); err != nil {
		return err
	}
	installer.mu.Lock()
	block := installer.block
	started := installer.started
	installer.started = nil
	installer.mu.Unlock()
	if started != nil {
		close(started)
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return os.WriteFile(filepath.Join(environmentPath, "python"), []byte("runtime"), 0o700)
}

type controlledSmokeTester struct {
	err   error
	calls atomic.Int32
}

func (tester *controlledSmokeTester) Test(context.Context, Profile, string) error {
	tester.calls.Add(1)
	return tester.err
}

type blockingSmokeTester struct {
	started chan struct{}
}

func (tester *blockingSmokeTester) Test(ctx context.Context, _ Profile, _ string) error {
	close(tester.started)
	<-ctx.Done()
	return ctx.Err()
}

type stubbornSmokeTester struct {
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (tester *stubbornSmokeTester) Test(context.Context, Profile, string) error {
	close(tester.started)
	<-tester.release
	close(tester.finished)
	return nil
}

func TestManagerDoesNotInstallBeforeExplicitInitialization(t *testing.T) {
	options, source, detector, downloader, installer, smoke := testManagerOptions(t)
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if source.calls.Load() != 0 || detector.calls.Load() != 0 || downloader.calls.Load() != 0 || installer.calls.Load() != 0 || smoke.calls.Load() != 0 {
		t.Fatal("manager performed installation work without explicit initialization")
	}
	if state := manager.State(context.Background()); state.LifecycleState != LifecycleNeedsInit {
		t.Fatalf("unexpected initial state %#v", state)
	}
}

func TestInitializationAuthorizationFailureIsPersistent(t *testing.T) {
	options, source, _, downloader, installer, smoke := testManagerOptions(t)
	source.err = errors.New("trusted manifest signature rejected")
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	job, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"})
	if err != nil || job.JobID == "" {
		t.Fatalf("initialization did not create a persistent job job=%#v error=%v", job, err)
	}
	state := waitForLifecycle(t, manager, LifecycleFailed)
	if state.InitializationJobID != job.JobID || !strings.Contains(state.Error, "signature rejected") {
		t.Fatalf("authorization failure was not exposed %#v", state)
	}
	var persisted InitializationJob
	if err := readJSONRegular(manager.jobPath(), &persisted, 1<<20); err != nil {
		t.Fatal(err)
	}
	if persisted.State != JobFailed || persisted.JobID != job.JobID {
		t.Fatalf("authorization failure was not persisted %#v", persisted)
	}
	if downloader.calls.Load() != 0 || installer.calls.Load() != 0 || smoke.calls.Load() != 0 {
		t.Fatal("authorization failure performed installation work")
	}
}

func TestManagerDeduplicatesInitializationAndPromotesAfterSmokeTest(t *testing.T) {
	options, _, _, downloader, installer, smoke := testManagerOptions(t)
	release := make(chan struct{})
	started := make(chan struct{})
	installer.block = release
	installer.started = started
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	first, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	second, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"})
	if err != nil {
		t.Fatal(err)
	}
	if first.JobID != second.JobID {
		t.Fatalf("duplicate request created new job %q != %q", first.JobID, second.JobID)
	}
	close(release)
	state := waitForLifecycle(t, manager, LifecycleReady)
	if state.RuntimeVersion != "0.10.0" || downloader.calls.Load() != 4 || installer.calls.Load() != 1 || smoke.calls.Load() != 1 {
		t.Fatalf("unexpected completed installation state=%#v calls=%d/%d/%d", state, downloader.calls.Load(), installer.calls.Load(), smoke.calls.Load())
	}
	manager.mu.Lock()
	activePath := manager.active.Path
	manager.mu.Unlock()
	if _, err := os.Stat(filepath.Join(activePath, "environment.json")); err != nil {
		t.Fatalf("promoted environment missing marker: %v", err)
	}
}

func TestCancellationPreservesPreviousEnvironment(t *testing.T) {
	options, _, _, _, installer, _ := testManagerOptions(t)
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"}); err != nil {
		t.Fatal(err)
	}
	waitForLifecycle(t, manager, LifecycleReady)
	manager.mu.Lock()
	previousPath := manager.active.Path
	manager.mu.Unlock()

	release := make(chan struct{})
	started := make(chan struct{})
	installer.mu.Lock()
	installer.block = release
	installer.started = started
	installer.mu.Unlock()
	job, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	cancelled, err := manager.CancelInitialization(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	if cancelled.JobID != job.JobID || cancelled.State != JobCancelled {
		t.Fatalf("unexpected cancelled job %#v", cancelled)
	}
	manager.mu.Lock()
	activePath := manager.active.Path
	manager.mu.Unlock()
	if activePath != previousPath {
		t.Fatalf("cancellation changed active environment %q to %q", previousPath, activePath)
	}
}

func TestCancellationDuringSmokeTestPreservesPersistentActiveEnvironment(t *testing.T) {
	options, _, _, _, _, _ := testManagerOptions(t)
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"}); err != nil {
		t.Fatal(err)
	}
	waitForLifecycle(t, manager, LifecycleReady)
	manager.mu.Lock()
	previous := manager.active
	manager.mu.Unlock()

	smoke := &blockingSmokeTester{started: make(chan struct{})}
	manager.options.SmokeTester = smoke
	job, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"})
	if err != nil {
		t.Fatal(err)
	}
	<-smoke.started
	if _, err := manager.CancelInitialization(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := waitForJobState(t, manager, job.JobID, JobCancelled)
	if state.LifecycleState != LifecycleReady && state.LifecycleState != LifecycleFailed {
		t.Fatalf("unexpected cancelled lifecycle %#v", state)
	}
	var persisted activeEnvironment
	if err := readJSONRegular(manager.activePath(), &persisted, 1<<20); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(persisted, previous) {
		t.Fatalf("cancellation changed persistent active environment %#v to %#v", previous, persisted)
	}
}

func TestSmokeFailureNeverPromotesEnvironment(t *testing.T) {
	options, _, _, _, _, smoke := testManagerOptions(t)
	smoke.err = errors.New("serve rejected")
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if _, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"}); err != nil {
		t.Fatal(err)
	}
	state := waitForLifecycle(t, manager, LifecycleFailed)
	if !strings.Contains(state.Error, "smoke test failed") {
		t.Fatalf("unexpected failure %#v", state)
	}
	manager.mu.Lock()
	activePath := manager.active.Path
	manager.mu.Unlock()
	if activePath != "" {
		t.Fatalf("failed environment was promoted to %q", activePath)
	}
}

func TestManagerRecoversPersistentRunningJob(t *testing.T) {
	options, _, _, _, installer, _ := testManagerOptions(t)
	now := time.Now().UTC()
	job := InitializationJob{JobID: "recovered-job", BackendID: BackendID, State: JobRunning, SelectedProfile: "cuda", Phase: "installing", TotalBytes: 2, CreatedAt: now, UpdatedAt: now}
	stateDirectory := filepath.Join(options.DataDir, "state")
	if err := ensurePrivateDirectory(stateDirectory); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(filepath.Join(stateDirectory, "initialization-job.json"), job, 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	state := waitForLifecycle(t, manager, LifecycleReady)
	if state.InitializationJobID != job.JobID || installer.calls.Load() != 1 {
		t.Fatalf("unexpected recovered state %#v calls=%d", state, installer.calls.Load())
	}
}

func TestManagerRecoveryClearsStalePartialArtifacts(t *testing.T) {
	options, _, _, downloader, _, _ := testManagerOptions(t)
	now := time.Now().UTC()
	job := InitializationJob{JobID: "recovered-partial", BackendID: BackendID, State: JobRunning, SelectedProfile: "cuda", Phase: "downloading", TotalBytes: 8, CreatedAt: now, UpdatedAt: now}
	stateDirectory := filepath.Join(options.DataDir, "state")
	if err := ensurePrivateDirectory(stateDirectory); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(filepath.Join(stateDirectory, "initialization-job.json"), job, 0o600); err != nil {
		t.Fatal(err)
	}
	staleArtifactDirectory := filepath.Join(options.DataDir, "staging", job.JobID, "artifacts")
	if err := ensurePrivateDirectory(staleArtifactDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleArtifactDirectory, "uv"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	state := waitForLifecycle(t, manager, LifecycleReady)
	if state.InitializationJobID != job.JobID || downloader.calls.Load() != 4 {
		t.Fatalf("unexpected recovered partial job %#v calls=%d", state, downloader.calls.Load())
	}
}

func TestManagerRejectsUnsafePersistentJobIdentity(t *testing.T) {
	options, _, _, _, _, _ := testManagerOptions(t)
	now := time.Now().UTC()
	job := InitializationJob{JobID: "../../escape", BackendID: BackendID, State: JobRunning, SelectedProfile: "cuda", Phase: "downloading", CreatedAt: now, UpdatedAt: now}
	stateDirectory := filepath.Join(options.DataDir, "state")
	if err := ensurePrivateDirectory(stateDirectory); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONAtomic(filepath.Join(stateDirectory, "initialization-job.json"), job, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(options); err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("unsafe persistent job was accepted: %v", err)
	}
}

func TestPersistenceFailureCancelsOnlyMatchingWorker(t *testing.T) {
	options, _, _, _, _, _ := testManagerOptions(t)
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	oldCancelled := atomic.Bool{}
	newCancelled := atomic.Bool{}
	manager.mu.Lock()
	manager.job = InitializationJob{JobID: "new-job", BackendID: BackendID, State: JobRunning, SelectedProfile: "cuda", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	manager.worker = &initializationWorker{jobID: "new-job", cancel: func() { newCancelled.Store(true) }}
	manager.failJobPersistenceLocked("old-job", errors.New("disk unavailable"))
	manager.clearWorkerLocked("old-job", true)
	if manager.worker == nil || manager.worker.jobID != "new-job" {
		t.Fatal("stale worker cleared replacement initialization")
	}
	manager.worker = &initializationWorker{jobID: "old-job", cancel: func() { oldCancelled.Store(true) }}
	manager.clearWorkerLocked("old-job", true)
	manager.mu.Unlock()
	if !oldCancelled.Load() || newCancelled.Load() {
		t.Fatalf("cancel handles crossed jobs old=%v new=%v", oldCancelled.Load(), newCancelled.Load())
	}
}

func TestProgressPersistenceFailureAbortsInitialization(t *testing.T) {
	options, _, _, downloader, installer, smoke := testManagerOptions(t)
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	manager.mu.Lock()
	originalWriter := manager.jobWriter
	writes := 0
	manager.jobWriter = func(path string, value any, mode os.FileMode) error {
		writes++
		if writes == 3 {
			return errors.New("disk unavailable")
		}
		return originalWriter(path, value, mode)
	}
	manager.mu.Unlock()
	if _, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"}); err != nil {
		t.Fatal(err)
	}
	state := waitForLifecycle(t, manager, LifecycleFailed)
	if state.InitializationPhase != "persistence_failed" || !strings.Contains(state.Error, "disk unavailable") {
		t.Fatalf("persistence failure was not surfaced: %#v", state)
	}
	if downloader.calls.Load() != 0 || installer.calls.Load() != 0 || smoke.calls.Load() != 0 {
		t.Fatalf("installation continued after persistence failure calls=%d/%d/%d", downloader.calls.Load(), installer.calls.Load(), smoke.calls.Load())
	}
}

func TestManagerCloseCancelsAndWaitsForInitializationCleanup(t *testing.T) {
	options, _, _, _, _, _ := testManagerOptions(t)
	smoke := &blockingSmokeTester{started: make(chan struct{})}
	options.SmokeTester = smoke
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"}); err != nil {
		t.Fatal(err)
	}
	<-smoke.started
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	jobState := manager.job.State
	manager.mu.Unlock()
	if jobState != JobCancelled {
		t.Fatalf("close returned before initialization cancellation persisted: %q", jobState)
	}
}

func TestManagerCloseHasBoundedInitializationWait(t *testing.T) {
	options, _, _, _, _, _ := testManagerOptions(t)
	smoke := &stubbornSmokeTester{started: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	options.SmokeTester = smoke
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	manager.closeWait = 50 * time.Millisecond
	if _, err := manager.StartInitialization(context.Background(), InitRequest{Profile: "cuda"}); err != nil {
		t.Fatal(err)
	}
	<-smoke.started
	startedAt := time.Now()
	closeError := manager.Close()
	if closeError == nil || !strings.Contains(closeError.Error(), "deadline exceeded") {
		t.Fatalf("bounded close did not report timeout: %v", closeError)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("bounded close took %s", elapsed)
	}
	close(smoke.release)
	select {
	case <-smoke.finished:
	case <-time.After(time.Second):
		t.Fatal("stubborn initialization did not finish after release")
	}
}

func testManagerOptions(t *testing.T) (ManagerOptions, *staticManifestSource, *staticDetector, *writingDownloader, *controlledInstaller, *controlledSmokeTester) {
	t.Helper()
	manifest := testManifest()
	digest := sha256.Sum256([]byte("uv"))
	manifest.Profiles[0].Artifacts[0].SHA256 = hex.EncodeToString(digest[:])
	source := &staticManifestSource{manifest: manifest}
	detector := &staticDetector{detection: Detection{OS: "linux", Architecture: "amd64", Devices: []string{"cuda", "cpu"}, Prerequisites: map[string]bool{"nvidia_driver": true}}}
	downloader := &writingDownloader{}
	installer := &controlledInstaller{}
	smoke := &controlledSmokeTester{}
	return ManagerOptions{DataDir: t.TempDir(), DefaultProfile: "auto", ManifestSource: source, Detector: detector, Downloader: downloader, Installer: installer, SmokeTester: smoke, DisableRecovery: false}, source, detector, downloader, installer, smoke
}

func waitForLifecycle(t *testing.T, manager *Manager, lifecycle string) State {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := manager.State(context.Background())
		if state.LifecycleState == lifecycle {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for lifecycle %q; state=%#v", lifecycle, manager.State(context.Background()))
	return State{}
}

func waitForJobState(t *testing.T, manager *Manager, jobID string, jobState string) State {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		state := manager.State(context.Background())
		manager.mu.Lock()
		matches := manager.job.JobID == jobID && manager.job.State == jobState
		manager.mu.Unlock()
		if matches {
			return state
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for job %q state %q", jobID, jobState)
	return State{}
}
