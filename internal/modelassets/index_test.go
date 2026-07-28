package modelassets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexCachesAndRevalidatesAsset(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "model.gguf")
	if err := os.WriteFile(path, []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := NewIndex(filepath.Join(root, "store"), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	asset, err := index.IndexFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found, ok := index.Find(asset.SHA256, "model.gguf")
	if !ok || found != path {
		t.Fatalf("asset lookup failed %q %v", found, ok)
	}
	if err := os.WriteFile(path, []byte("different"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := index.Find(asset.SHA256, "model.gguf"); ok {
		t.Fatal("changed asset passed revalidation")
	}
}

func TestIndexRejectsUnsafeFilenameBeforeFileAccess(t *testing.T) {
	index, err := NewIndex(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	_, err = index.IndexFile(filepath.Join(t.TempDir(), "CON"))
	if err == nil || !strings.Contains(err.Error(), "unsafe filename") {
		t.Fatalf("unsafe filename was not rejected at the index boundary: %v", err)
	}
}

func TestResolutionJobsAndOriginsSurviveRestart(t *testing.T) {
	root := t.TempDir()
	index, err := NewIndex(root, "")
	if err != nil {
		t.Fatal(err)
	}
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	origin := Origin{Repository: "owner/model", Commit: "0123456789abcdef0123456789abcdef01234567", Path: "model.gguf"}
	if err := index.BindOrigin(hash, origin); err != nil {
		t.Fatal(err)
	}
	job, err := index.CreateResolutionJob("config", "node")
	if err != nil {
		t.Fatal(err)
	}
	job.State = JobFailed
	job.Error = "model asset unavailable"
	job.Results = []FieldResult{{Field: "model_param", Hash: hash, Failure: "asset unavailable", Source: "peer", Verification: "sha256"}}
	if err := index.UpdateResolutionJob(job); err != nil {
		t.Fatal(err)
	}
	if err := index.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewIndex(root, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if persisted, found := reopened.Origin(hash); !found || persisted != origin {
		t.Fatalf("origin did not survive restart %#v", persisted)
	}
	persistedJob, found, err := reopened.ResolutionJob(job.ID)
	if err != nil || !found || persistedJob.State != JobFailed || len(persistedJob.Results) != 1 || persistedJob.Results[0].Verification != "sha256" {
		t.Fatalf("job did not survive restart %#v found=%v error=%v", persistedJob, found, err)
	}
}
