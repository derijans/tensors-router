package downloader

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestInterruptedJobResumesFromStagedBytesAfterRestart(t *testing.T) {
	content := []byte("0123456789")
	var resumedFrom atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		resumedFrom.Store(request.Header.Get("Range"))
		writer.Header().Set("Content-Range", "bytes 6-9/10")
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = writer.Write(content[6:])
	}))
	defer server.Close()
	directory := t.TempDir()
	config := DefaultConfig(filepath.Join(directory, "downloader.yaml"))
	config.Logging.Mode = "off"
	config.Storage.FreeSpaceReserveGB = 0
	first, err := NewManager(config, "")
	if err != nil {
		t.Fatal(err)
	}
	job := DownloadJob{ID: "interrupted", Repository: "owner/model", Commit: "commit", State: JobRunning, TotalBytes: 10, Files: []JobFile{{Path: "model.gguf", Size: 10, State: string(JobRunning)}}}
	if err := first.store.SaveJob(job); err != nil {
		t.Fatal(err)
	}
	staging, err := first.stagingDirectory(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "model.gguf"), content[:6], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewManager(config, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	restarted.hub.baseURL = server.URL + "/api"
	restarted.RecoverInterrupted()
	completed := waitForJobState(t, restarted, job.ID, JobCompleted)
	if resumedFrom.Load() != "bytes=6-" {
		t.Fatalf("restart did not resume from the staged offset: %v", resumedFrom.Load())
	}
	if completed.CompletedBytes != 10 {
		t.Fatalf("unexpected completed bytes %d", completed.CompletedBytes)
	}
	destination, err := DestinationPath(config.Storage.Root, job.Repository, "model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if promoted, err := os.ReadFile(destination); err != nil || string(promoted) != string(content) {
		t.Fatalf("resumed file was not promoted intact: %q %v", promoted, err)
	}
}

func TestPauseAfterQuickResumeStillCancelsTheNewTransfer(t *testing.T) {
	var requests atomic.Int32
	started := make(chan int32, 4)
	cancelled := make(chan int32, 4)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		number := requests.Add(1)
		started <- number
		<-request.Context().Done()
		cancelled <- number
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	job := queuedTestJob(t, manager, "pause-resume", 5)
	if err := manager.startJob(job.ID); err != nil {
		t.Fatal(err)
	}
	expectSignal(t, started, 1, "first transfer did not start")
	if _, err := manager.Pause(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Resume(job.ID); err != nil {
		t.Fatal(err)
	}
	expectSignal(t, started, 2, "resumed transfer did not start")
	if state := waitForJobState(t, manager, job.ID, JobRunning).State; state != JobRunning {
		t.Fatalf("quick resume left the job %s", state)
	}
	if _, err := manager.Pause(job.ID); err != nil {
		t.Fatal(err)
	}
	expectSignal(t, cancelled, 1, "first transfer was not cancelled")
	expectSignal(t, cancelled, 2, "pausing after a quick resume did not cancel the resumed transfer")
	if paused, _, _ := manager.store.Job(job.ID); paused.State != JobPaused {
		t.Fatalf("job ended %s instead of paused", paused.State)
	}
}

func TestCancelRemovesStagedBytes(t *testing.T) {
	manager := transferTestManager(t, "http://127.0.0.1:1/api")
	job := queuedTestJob(t, manager, "cancel-staging", 5)
	if _, err := manager.store.TransitionJob(job.ID, JobPaused, "", JobQueued); err != nil {
		t.Fatal(err)
	}
	staging, err := manager.stagingDirectory(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "model.gguf"), []byte("01"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("cancelled job left staging data behind: %v", err)
	}
}

func TestRepeatedCreateJoinsTheActiveJob(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	plan := DownloadPlan{Repository: "owner/model", Revision: "main", Commit: "commit", TotalBytes: 5, Files: []PlannedFile{{Path: "model.gguf", Size: 5}}}
	first, err := manager.CreatePlannedJob(plan, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.CreatePlannedJob(plan, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("identical request started a second parallel job: %s and %s", first.ID, second.ID)
	}
}

func TestEmptyRepositoryFileDownloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", "0")
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	plan := DownloadPlan{Repository: "owner/package", Revision: "main", Commit: "commit", Files: []PlannedFile{{Path: "__init__.py", Size: 0}}}
	job, err := manager.CreatePlannedJob(plan, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	waitForJobState(t, manager, job.ID, JobCompleted)
}

func queuedTestJob(t *testing.T, manager *Manager, id string, size int64) DownloadJob {
	t.Helper()
	job := DownloadJob{ID: id, Repository: "owner/model", Commit: "commit", State: JobQueued, TotalBytes: size, Files: []JobFile{{Path: "model.gguf", Size: size, State: string(JobQueued)}}}
	if err := manager.store.SaveJob(job); err != nil {
		t.Fatal(err)
	}
	return job
}

func waitForJobState(t *testing.T, manager *Manager, id string, want JobState) DownloadJob {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		job, found, err := manager.store.Job(id)
		if err != nil || !found {
			t.Fatalf("job %s unavailable: found=%t error=%v", id, found, err)
		}
		if job.State == want {
			return job
		}
		if job.State.Terminal() || time.Now().After(deadline) {
			t.Fatalf("job %s reached %s (error %q) instead of %s", id, job.State, job.Error, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func expectSignal(t *testing.T, signals <-chan int32, want int32, failure string) {
	t.Helper()
	select {
	case got := <-signals:
		if got != want {
			t.Fatalf("%s: got request %d, want %d", failure, got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal(failure)
	}
}
