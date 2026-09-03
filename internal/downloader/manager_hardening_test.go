package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPlanForEmptyRepositoryIsRejectedAndTouchesNoDirectory(t *testing.T) {
	root := t.TempDir()
	details := RepositoryDetails{Repository: "owner/model", Revision: "main", Commit: "0123456789abcdef0123456789abcdef01234567", Files: nil}

	plan, err := BuildPlan(details, nil, "smart", root)
	if err != nil {
		t.Fatalf("planning an empty repository should still succeed: %v", err)
	}
	if len(plan.Files) != 0 {
		t.Fatalf("expected zero planned files, got %d", len(plan.Files))
	}
	if _, err := os.Stat(filepath.Join(root, "owner")); !os.IsNotExist(err) {
		t.Fatalf("planning created a repository directory on disk: %v", err)
	}

	manager := &Manager{config: Config{Storage: StorageConfig{Root: root}}}
	if _, err := manager.CreatePlannedJob(plan, "", false, false); err == nil {
		t.Fatal("a plan with zero files must be rejected")
	}
	if _, err := os.Stat(filepath.Join(root, "owner")); !os.IsNotExist(err) {
		t.Fatalf("a rejected job created a repository directory on disk: %v", err)
	}
}

func TestDownloadDestinationResolveDoesNotCreateDirectories(t *testing.T) {
	root := t.TempDir()
	resolved, err := downloadDestinationResolve(root, "owner/model", "commit", false, "weights/model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if !pathWithin(resolved, root) {
		t.Fatalf("resolved path escaped the storage root: %q", resolved)
	}
	if _, err := os.Stat(filepath.Join(root, "owner")); !os.IsNotExist(err) {
		t.Fatalf("resolving a destination created directories: %v", err)
	}
}

func TestDownloadDestinationPathStillCreatesDirectories(t *testing.T) {
	root := t.TempDir()
	if _, err := downloadDestinationPath(root, "owner/model", "commit", false, "weights/model.gguf"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "owner", "model", "weights")); err != nil {
		t.Fatalf("the write path helper did not create the destination tree: %v", err)
	}
}

func TestRecoverInterruptedFailsUnfinishedJobs(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "downloads.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	running := DownloadJob{ID: "running", Repository: "owner/model", Commit: "commit", State: JobRunning, Files: []JobFile{{Path: "a.gguf", Size: 1, State: string(JobRunning)}}}
	done := DownloadJob{ID: "done", Repository: "owner/model", Commit: "commit", State: JobCompleted, Files: []JobFile{{Path: "b.gguf", Size: 1, State: string(JobCompleted)}}}
	for _, job := range []DownloadJob{running, done} {
		if err := store.SaveJob(job); err != nil {
			t.Fatal(err)
		}
	}

	affected, err := store.RecoverInterrupted("router stopped")
	if err != nil {
		t.Fatal(err)
	}
	if affected != 1 {
		t.Fatalf("expected exactly one interrupted job, got %d", affected)
	}

	recovered, _, err := store.Job("running")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.State != JobFailed || recovered.Error != "router stopped" {
		t.Fatalf("interrupted job was not marked failed: %#v", recovered)
	}
	if recovered.Files[0].State != string(JobFailed) {
		t.Fatalf("interrupted job file was not marked failed: %#v", recovered.Files[0])
	}

	untouched, _, err := store.Job("done")
	if err != nil {
		t.Fatal(err)
	}
	if untouched.State != JobCompleted {
		t.Fatalf("a completed job must not be recovered: %#v", untouched)
	}
}
