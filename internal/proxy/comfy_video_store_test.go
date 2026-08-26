package proxy

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestComfyVideoStore(t *testing.T) *comfyVideoJobStore {
	t.Helper()
	return newComfyVideoJobStore(t.TempDir())
}

// A finished video must live on disk rather than in the router process: a
// single generation runs to hundreds of megabytes and completed jobs are kept
// for a day, so holding them resident would let a few concurrent jobs exhaust
// the process.
func TestComfyVideoOutputIsWrittenToDiskAndServedFromThere(t *testing.T) {
	store := newTestComfyVideoStore(t)
	_, job, err := store.create()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("finished-video-bytes")
	size, err := store.writeVideo(job, func(target io.Writer) error {
		_, writeErr := target.Write(payload)
		return writeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	job.complete(size)

	snapshot := job.snapshot()
	if snapshot.size != int64(len(payload)) {
		t.Fatalf("recorded size %d, want %d", snapshot.size, len(payload))
	}
	onDisk, err := os.ReadFile(snapshot.path)
	if err != nil {
		t.Fatalf("finished video was not written to disk: %v", err)
	}
	if string(onDisk) != string(payload) {
		t.Fatalf("on-disk video = %q, want %q", onDisk, payload)
	}
}

func TestComfyVideoWriteRejectsOutputOverTheSizeCapAndLeavesNoFile(t *testing.T) {
	store := newTestComfyVideoStore(t)
	store.maxVideoBytes = 16
	_, job, err := store.create()
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.writeVideo(job, func(target io.Writer) error {
		_, writeErr := target.Write(make([]byte, 64))
		return writeErr
	})
	if !errors.Is(err, errComfyVideoTooLarge) {
		t.Fatalf("oversized video error = %v, want errComfyVideoTooLarge", err)
	}
	if _, statErr := os.Stat(job.path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("a rejected video must not leave bytes behind, stat = %v", statErr)
	}
}

func TestComfyVideoStoreEvictsOldestJobBeyondTheJobLimit(t *testing.T) {
	store := newTestComfyVideoStore(t)
	base := time.Now()
	created := make([]string, 0, maxComfyVideoJobs+1)
	for index := 0; index <= maxComfyVideoJobs; index++ {
		moment := base.Add(time.Duration(index) * time.Second)
		store.now = func() time.Time { return moment }
		id, _, err := store.create()
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, id)
	}
	store.now = func() time.Time { return base }

	if _, ok := store.get(created[0]); ok {
		t.Fatal("the oldest job should have been evicted at the job limit")
	}
	if _, ok := store.get(created[len(created)-1]); !ok {
		t.Fatal("the newest job should still be present")
	}
}

func TestComfyVideoStoreExpiresJobsAndDeletesTheirFiles(t *testing.T) {
	store := newTestComfyVideoStore(t)
	now := time.Now()
	store.now = func() time.Time { return now }
	id, job, err := store.create()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.writeVideo(job, func(target io.Writer) error {
		_, writeErr := target.Write([]byte("expiring"))
		return writeErr
	}); err != nil {
		t.Fatal(err)
	}

	now = now.Add(comfyVideoJobLifetime + time.Minute)
	if _, ok := store.get(id); ok {
		t.Fatal("an expired job must not resolve")
	}
	if _, statErr := os.Stat(job.path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("an expired job's file must be deleted, stat = %v", statErr)
	}
}

// A stored upload is keyed by the backend's name but written to a path derived
// from a hash of it, so a traversal-shaped name cannot escape the scratch
// directory.
func TestComfyVideoStoreUploadKeepsHostileNamesInsideTheScratchDirectory(t *testing.T) {
	store := newTestComfyVideoStore(t)
	hostile := `../../../../etc/passwd`
	if err := store.rememberUpload(hostile, []byte("reference-image")); err != nil {
		t.Fatal(err)
	}

	content, ok := store.uploadBytes(hostile)
	if !ok || string(content) != "reference-image" {
		t.Fatalf("upload did not round-trip: ok=%t content=%q", ok, content)
	}
	stored := store.uploads[hostile].path
	if filepath.Dir(stored) != store.dir {
		t.Fatalf("upload escaped the scratch directory: %q is not inside %q", stored, store.dir)
	}
	if strings.ContainsAny(filepath.Base(stored), `/\`) {
		t.Fatalf("stored upload name carries a path separator: %q", filepath.Base(stored))
	}
}

func TestComfyVideoStoreRejectsUploadOverTheSizeCap(t *testing.T) {
	store := newTestComfyVideoStore(t)
	err := store.rememberUpload("big.png", make([]byte, maxComfyVideoUploadBytes+1))
	if err == nil || !strings.Contains(err.Error(), "size cap") {
		t.Fatalf("oversized upload error = %v, want a size cap rejection", err)
	}
	if _, ok := store.uploadBytes("big.png"); ok {
		t.Fatal("a rejected upload must not be retrievable")
	}
}

func TestComfyVideoStoreReportsAnUnusableScratchDirectory(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newComfyVideoJobStore(blocker)

	_, _, err := store.create()
	if err == nil {
		t.Fatal("expected job creation to fail when the scratch directory cannot be created")
	}
	if !strings.Contains(err.Error(), "scratch directory") {
		t.Fatalf("error %q does not name the scratch directory", err)
	}
}

func TestCappedWriterStopsAtTheLimit(t *testing.T) {
	overflow := fmt.Errorf("over")
	writer := &cappedWriter{target: io.Discard, remaining: 8, overflow: overflow}
	if _, err := writer.Write(make([]byte, 8)); err != nil {
		t.Fatalf("writing exactly the limit failed: %v", err)
	}
	if _, err := writer.Write([]byte{1}); !errors.Is(err, overflow) {
		t.Fatalf("write past the limit = %v, want overflow", err)
	}
	if writer.written != 8 {
		t.Fatalf("written = %d, want 8", writer.written)
	}
}
