package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeDownloadResumesWithAuthorizedRangeRequest(t *testing.T) {
	content := []byte("abcdefghij")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/owner/model/resolve/commit/model.gguf" || request.URL.Query().Get("download") != "true" {
			http.Error(writer, "unexpected path", http.StatusNotFound)
			return
		}
		if request.Header.Get("Authorization") != "Bearer gated-token" || request.Header.Get("Range") != "bytes=4-" {
			http.Error(writer, "missing authorization or range", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Range", "bytes 4-9/10")
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = writer.Write(content[4:])
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	stagingPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(stagingPath, content[:4], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.downloadFile(context.Background(), "owner/model", "commit", "model.gguf", stagingPath, int64(len(content)), "gated-token"); err != nil {
		t.Fatal(err)
	}
	downloaded, err := os.ReadFile(stagingPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != string(content) {
		t.Fatalf("unexpected resumed content %q", downloaded)
	}
}

func TestNativeDownloadRetriesTransientFailure(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(writer, "retry", http.StatusServiceUnavailable)
			return
		}
		_, _ = writer.Write([]byte("model"))
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	manager.config.Downloads.RetryLimit = 1
	stagingPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := manager.downloadFile(context.Background(), "owner/model", "commit", "model.gguf", stagingPath, 5, ""); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("unexpected request count %d", requests.Load())
	}
}

func TestNativeDownloadDoesNotForwardTokenAcrossRedirect(t *testing.T) {
	var leakedToken atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		leakedToken.Store(request.Header.Get("Authorization") != "")
		_, _ = writer.Write([]byte("model"))
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer gated-token" {
			http.Error(writer, "missing token", http.StatusUnauthorized)
			return
		}
		http.Redirect(writer, request, target.URL+"/asset", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	manager := transferTestManager(t, origin.URL+"/api")
	if err := manager.downloadFile(context.Background(), "owner/model", "commit", "model.gguf", filepath.Join(t.TempDir(), "model.gguf"), 5, "gated-token"); err != nil {
		t.Fatal(err)
	}
	if leakedToken.Load() {
		t.Fatal("gated token was forwarded to the redirect host")
	}
}

func TestTransferRejectsHashMismatchBeforePromotion(t *testing.T) {
	content := []byte("tampered")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write(content)
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	expected := sha256.Sum256([]byte("expected"))
	job := DownloadJob{
		ID:         "hash-mismatch",
		Repository: "owner/model",
		Commit:     "commit",
		State:      JobRunning,
		TotalBytes: int64(len(content)),
		Files:      []JobFile{{Path: "model.gguf", Size: int64(len(content)), ExpectedSHA256: hex.EncodeToString(expected[:]), State: string(JobQueued)}},
	}
	if err := manager.store.SaveJob(job); err != nil {
		t.Fatal(err)
	}
	stored, _, err := manager.store.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.transfer(context.Background(), stored); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unexpected hash result %v", err)
	}
	destination, err := DestinationPath(manager.config.Storage.Root, job.Repository, "model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("hash-mismatched file was promoted: %v", err)
	}
}

func transferTestManager(t *testing.T, baseURL string) *Manager {
	t.Helper()
	directory := t.TempDir()
	config := DefaultConfig(filepath.Join(directory, "downloader.yaml"))
	config.Logging.Mode = "off"
	config.Storage.FreeSpaceReserveGB = 0
	manager, err := NewManager(config, "")
	if err != nil {
		t.Fatal(err)
	}
	manager.hub.baseURL = baseURL
	t.Cleanup(func() { _ = manager.Close() })
	return manager
}
