package downloader

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreSavesAndLoadsJob(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "downloads.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	want := DownloadJob{
		ID:             "job-1",
		Repository:     "owner/model",
		Revision:       "main",
		Commit:         "0123456789abcdef",
		State:          JobQueued,
		TotalBytes:     128,
		CompletedBytes: 0,
		Snapshot:       true,
		TreeSHA256:     strings.Repeat("a", 64),
		Files: []JobFile{{
			Path:           "model.gguf",
			Reason:         "requested",
			ExpectedSHA256: "",
			Size:           128,
			State:          string(JobQueued),
		}},
	}
	if err := store.SaveJob(want); err != nil {
		t.Fatal(err)
	}

	got, found, err := store.Job(want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got.Commit != want.Commit || !got.Snapshot || got.TreeSHA256 != want.TreeSHA256 || len(got.Files) != 1 || got.Files[0].Path != want.Files[0].Path {
		t.Fatalf("unexpected stored job %#v", got)
	}
}

func TestOpenStoreMigratesLegacyCommitColumn(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "downloads.sqlite")
	legacy, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE jobs (id TEXT PRIMARY KEY, repository TEXT NOT NULL, revision TEXT NOT NULL, "commit" TEXT NOT NULL, state TEXT NOT NULL, total_bytes INTEGER NOT NULL, completed_bytes INTEGER NOT NULL, error TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	job := DownloadJob{ID: "job-legacy", Repository: "owner/model", Revision: "main", Commit: "0123456789abcdef", State: JobQueued}
	if err := store.SaveJob(job); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Job(job.ID); err != nil || !found {
		t.Fatalf("legacy job migration failed found=%t err=%v", found, err)
	}
}
