package downloader

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

const libraryTestCommit = "0123456789abcdef0123456789abcdef01234567"

func TestRescanReturnsPersistedTimestamps(t *testing.T) {
	manager := transferTestManager(t, "http://hub.invalid")
	writeLibraryFile(t, filepath.Join(manager.config.Storage.Root, "loose.gguf"), "loose weights")

	artifacts, err := manager.Rescan()
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].CreatedAt.IsZero() || artifacts[0].UpdatedAt.IsZero() {
		t.Fatalf("rescan returned records without their stored timestamps %#v", artifacts)
	}
}

func TestPlanSkipsFileAlreadyPresentWithTheSameHash(t *testing.T) {
	weights := "identical weights"
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"sha":%q,"siblings":[{"rfilename":"model.gguf","lfs":{"oid":%q,"size":%d}},{"rfilename":"config.json","size":2}]}`, libraryTestCommit, sha256Text(t, weights), len(weights))
	}))
	defer hub.Close()
	manager := transferTestManager(t, hub.URL)
	destination, err := downloadDestinationPath(manager.config.Storage.Root, "owner/model", libraryTestCommit, false, "model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	writeLibraryFile(t, destination, weights)
	if _, err := manager.Rescan(); err != nil {
		t.Fatal(err)
	}

	plan, err := manager.Plan(context.Background(), PlanRequest{Repository: "owner/model", Mode: "explicit", Files: []string{"model.gguf", "config.json"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files) != 1 || plan.Files[0].Path != "config.json" || plan.TotalBytes != 2 {
		t.Fatalf("present file was planned again %#v", plan)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0] != (SkippedFile{Path: "model.gguf", Reason: alreadyPresentReason}) {
		t.Fatalf("present file was not reported as skipped %#v", plan.Skipped)
	}
}

func TestKnownArtifactScanNeverHashesUnknownFiles(t *testing.T) {
	manager := transferTestManager(t, "http://hub.invalid")
	known := filepath.Join(manager.config.Storage.Root, "known.gguf")
	writeLibraryFile(t, known, "known weights")
	if err := WriteHashSidecar(known, sha256Text(t, "known weights")); err != nil {
		t.Fatal(err)
	}
	unknown := filepath.Join(manager.config.Storage.Root, "unknown.gguf")
	writeLibraryFile(t, unknown, "unknown weights")

	if err := manager.ScanKnownArtifacts(context.Background()); err != nil {
		t.Fatal(err)
	}

	artifacts, err := manager.Artifacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].Path != known {
		t.Fatalf("known scan recorded %#v", artifacts)
	}
	unhashed, err := manager.Unhashed()
	if err != nil {
		t.Fatal(err)
	}
	if len(unhashed) != 1 || unhashed[0] != (UnhashedFile{Path: unknown, Size: int64(len("unknown weights"))}) {
		t.Fatalf("unknown file was not listed as unhashed %#v", unhashed)
	}

	if _, err := manager.Rescan(); err != nil {
		t.Fatal(err)
	}
	if unhashed, _ := manager.Unhashed(); len(unhashed) != 0 {
		t.Fatalf("rescan left hashed files listed as unhashed %#v", unhashed)
	}
}

func writeLibraryFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func sha256Text(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "content")
	writeLibraryFile(t, path, content)
	hash, _, err := SHA256File(path)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestKnownArtifactScanKeepsTheProvenanceOfDownloadedFiles(t *testing.T) {
	manager := transferTestManager(t, "http://hub.invalid")
	path := filepath.Join(manager.config.Storage.Root, "owner", "model", "model.gguf")
	writeLibraryFile(t, path, "downloaded weights")
	hash := sha256Text(t, "downloaded weights")
	if err := WriteHashSidecar(path, hash); err != nil {
		t.Fatal(err)
	}
	downloaded, err := artifactFromFile(path, hash, "owner/model", "model.gguf", libraryTestCommit, "download")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.recordArtifact(downloaded); err != nil {
		t.Fatal(err)
	}

	if err := manager.ScanKnownArtifacts(context.Background()); err != nil {
		t.Fatal(err)
	}

	record, found, err := manager.store.Artifact(path)
	if err != nil || !found || record.Repository != "owner/model" || record.Revision != libraryTestCommit || record.VerificationSource != "download" {
		t.Fatalf("start scan rewrote the downloaded artifact found=%t error=%v record=%#v", found, err, record)
	}
}
