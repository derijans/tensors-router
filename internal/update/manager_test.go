package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"tensors-router/internal/config"
)

func TestDownloadWritesBinaryAndMetadata(t *testing.T) {
	cfg := testConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		_, _ = w.Write([]byte("binary"))
	}))
	defer server.Close()
	cfg.Updates.BinaryURL = server.URL

	manager := NewManager(cfg)
	manager.Now = func() time.Time { return time.Unix(100, 0).UTC() }

	if err := manager.Download(context.Background()); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(cfg.Kobold.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "binary" {
		t.Fatalf("unexpected binary content %q", string(content))
	}

	meta := manager.readMetadata()
	if meta.ETag != `"abc"` {
		t.Fatalf("unexpected etag %q", meta.ETag)
	}
}

func TestEnsureSkipsFreshCheck(t *testing.T) {
	cfg := testConfig(t)
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("binary"))
	}))
	defer server.Close()
	cfg.Updates.BinaryURL = server.URL

	if err := os.MkdirAll(filepath.Dir(cfg.Kobold.BinaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Kobold.BinaryPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(cfg)
	now := time.Unix(200, 0).UTC()
	manager.Now = func() time.Time { return now }
	if err := manager.writeMetadata(metadata{
		CheckedAt: now.Add(-time.Hour),
		URL:       cfg.Updates.BinaryURL,
	}); err != nil {
		t.Fatal(err)
	}

	if err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 0 {
		t.Fatalf("expected no server hits, got %d", hits.Load())
	}
}

func TestDownloadFailureKeepsPreviousBinary(t *testing.T) {
	cfg := testConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()
	cfg.Updates.BinaryURL = server.URL

	if err := os.MkdirAll(filepath.Dir(cfg.Kobold.BinaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Kobold.BinaryPath, []byte("previous"), 0o755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(cfg)
	if err := manager.Download(context.Background()); err == nil {
		t.Fatalf("expected download failure")
	}

	content, err := os.ReadFile(cfg.Kobold.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "previous" {
		t.Fatalf("unexpected binary content %q", string(content))
	}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Kobold.BinaryPath = filepath.Join(dir, "bin", "koboldcpp")
	cfg.Kobold.DataDir = filepath.Join(dir, "data")
	cfg.Updates.CheckInterval = 168 * time.Hour
	return cfg
}
