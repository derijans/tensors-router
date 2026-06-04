package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		_, _ = w.Write([]byte("binary"))
	}))
	defer server.Close()
	cfg.Updates.BinaryURL = server.URL
	cfg.Updates.BinarySHA256 = sha256Hex("binary")

	manager := NewManager(cfg)
	manager.client = server.Client()
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

	meta := manager.readMetadata(manager.targets()[0])
	if meta.ETag != `"abc"` {
		t.Fatalf("unexpected etag %q", meta.ETag)
	}
	if meta.SHA256 != cfg.Updates.BinarySHA256 {
		t.Fatalf("unexpected sha256 %q", meta.SHA256)
	}
}

func TestEnsureSkipsFreshCheck(t *testing.T) {
	cfg := testConfig(t)
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("binary"))
	}))
	defer server.Close()
	cfg.Updates.BinaryURL = server.URL
	cfg.Updates.BinarySHA256 = sha256Hex("old")

	if err := os.MkdirAll(filepath.Dir(cfg.Kobold.BinaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Kobold.BinaryPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(cfg)
	manager.client = server.Client()
	now := time.Unix(200, 0).UTC()
	manager.Now = func() time.Time { return now }
	if err := manager.writeMetadata(manager.targets()[0], metadata{
		CheckedAt: now.Add(-time.Hour),
		URL:       cfg.Updates.BinaryURL,
		SHA256:    cfg.Updates.BinarySHA256,
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
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()
	cfg.Updates.BinaryURL = server.URL
	cfg.Updates.BinarySHA256 = sha256Hex("binary")

	if err := os.MkdirAll(filepath.Dir(cfg.Kobold.BinaryPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Kobold.BinaryPath, []byte("previous"), 0o755); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(cfg)
	manager.client = server.Client()
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

func TestDownloadSplitModeWritesLlamaAndSDCPPBinaries(t *testing.T) {
	cfg := testConfig(t)
	cfg.Backend.Mode = "llama_sdcpp"
	binRoot := filepath.Dir(filepath.Dir(cfg.Kobold.BinaryPath))
	cfg.Llama.BinaryPath = filepath.Join(binRoot, "llama", "llama-server")
	cfg.Llama.DataDir = filepath.Join(filepath.Dir(cfg.Kobold.DataDir), "llama")
	cfg.SDCPP.BinaryPath = filepath.Join(binRoot, "stable-diffusion", "sd-server")
	cfg.SDCPP.DataDir = filepath.Join(filepath.Dir(cfg.Kobold.DataDir), "sdcpp")

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/llama":
			_, _ = w.Write([]byte("llama"))
		case "/sdcpp":
			_, _ = w.Write([]byte("sdcpp"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg.Updates.LlamaBinaryURL = server.URL + "/llama"
	cfg.Updates.LlamaSHA256 = sha256Hex("llama")
	cfg.Updates.SDCPPBinaryURL = server.URL + "/sdcpp"
	cfg.Updates.SDCPPSHA256 = sha256Hex("sdcpp")

	manager := NewManager(cfg)
	manager.client = server.Client()
	paths, err := manager.DownloadedPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != cfg.Llama.BinaryPath || paths[1] != cfg.SDCPP.BinaryPath {
		t.Fatalf("unexpected downloaded paths %#v", paths)
	}

	llamaContent, err := os.ReadFile(cfg.Llama.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(llamaContent) != "llama" {
		t.Fatalf("unexpected llama binary %q", string(llamaContent))
	}
	sdcppContent, err := os.ReadFile(cfg.SDCPP.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(sdcppContent) != "sdcpp" {
		t.Fatalf("unexpected sdcpp binary %q", string(sdcppContent))
	}

	if manager.readMetadata(manager.targets()[0]).URL != cfg.Updates.LlamaBinaryURL {
		t.Fatalf("llama metadata was not written")
	}
	if manager.readMetadata(manager.targets()[1]).URL != cfg.Updates.SDCPPBinaryURL {
		t.Fatalf("sdcpp metadata was not written")
	}
}

func TestDownloadSplitModeExtractsArchivedBinaries(t *testing.T) {
	cfg := testConfig(t)
	cfg.Backend.Mode = "llama_sdcpp"
	cfg.Llama.BinaryPath = filepath.Join(filepath.Dir(filepath.Dir(cfg.Kobold.BinaryPath)), "llama", "llama-b9495", "llama-server")
	cfg.Llama.DataDir = filepath.Join(filepath.Dir(cfg.Kobold.DataDir), "llama")
	cfg.SDCPP.BinaryPath = filepath.Join(filepath.Dir(filepath.Dir(cfg.Kobold.BinaryPath)), "stable-diffusion", "build", "bin", "sd-server")
	cfg.SDCPP.DataDir = filepath.Join(filepath.Dir(cfg.Kobold.DataDir), "sdcpp")
	sdcppRoot := filepath.Join(filepath.Dir(filepath.Dir(cfg.Kobold.BinaryPath)), "stable-diffusion")
	if err := os.MkdirAll(sdcppRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdcppRoot, "stale-runtime.so"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	llamaArchive := tarGzPayload(t, []archiveFile{
		{Name: "llama-b9495/llama-server", Content: "llama"},
		{Name: "llama-b9495/libllama.so", Content: "llama-lib"},
	})
	sdcppArchive := zipPayload(t, []archiveFile{
		{Name: "build/bin/sd-server", Content: "sdcpp"},
		{Name: "build/bin/sd-cli", Content: "sdcpp-cli"},
		{Name: "build/bin/libstable-diffusion.so", Content: "sdcpp-lib"},
		{Name: "build/bin/stable-diffusion.cpp.txt", Content: "sdcpp-license"},
	})

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/llama.tar.gz":
			_, _ = w.Write(llamaArchive)
		case "/sdcpp.zip":
			_, _ = w.Write(sdcppArchive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg.Updates.LlamaBinaryURL = server.URL + "/llama.tar.gz"
	cfg.Updates.LlamaSHA256 = sha256BytesHex(llamaArchive)
	cfg.Updates.SDCPPBinaryURL = server.URL + "/sdcpp.zip"
	cfg.Updates.SDCPPSHA256 = sha256BytesHex(sdcppArchive)

	manager := NewManager(cfg)
	manager.client = server.Client()
	if err := manager.Download(context.Background()); err != nil {
		t.Fatal(err)
	}

	llamaContent, err := os.ReadFile(cfg.Llama.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(llamaContent) != "llama" {
		t.Fatalf("unexpected llama binary %q", string(llamaContent))
	}
	sdcppContent, err := os.ReadFile(cfg.SDCPP.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(sdcppContent) != "sdcpp" {
		t.Fatalf("unexpected sdcpp binary %q", string(sdcppContent))
	}
	assertFileContent(t, filepath.Join(filepath.Dir(cfg.Llama.BinaryPath), "libllama.so"), "llama-lib")
	assertFileContent(t, filepath.Join(filepath.Dir(cfg.SDCPP.BinaryPath), "libstable-diffusion.so"), "sdcpp-lib")
	assertFileContent(t, filepath.Join(filepath.Dir(cfg.SDCPP.BinaryPath), "sd-cli"), "sdcpp-cli")
	assertFileContent(t, filepath.Join(filepath.Dir(cfg.SDCPP.BinaryPath), "stable-diffusion.cpp.txt"), "sdcpp-license")
	if fileExists(filepath.Join(sdcppRoot, "stale-runtime.so")) {
		t.Fatalf("stale archive content was not removed")
	}

	if manager.readMetadata(manager.targets()[0]).BinarySHA256 != sha256Hex("llama") {
		t.Fatalf("llama metadata did not record extracted binary hash")
	}
	if manager.readMetadata(manager.targets()[1]).BinarySHA256 != sha256Hex("sdcpp") {
		t.Fatalf("sdcpp metadata did not record extracted binary hash")
	}
}

func TestDownloadRejectsHTTPURL(t *testing.T) {
	cfg := testConfig(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("binary"))
	}))
	defer server.Close()
	cfg.Updates.BinaryURL = server.URL
	cfg.Updates.BinarySHA256 = sha256Hex("binary")

	if err := NewManager(cfg).Download(context.Background()); err == nil {
		t.Fatalf("expected http url rejection")
	}
}

func TestDownloadRejectsSHA256Mismatch(t *testing.T) {
	cfg := testConfig(t)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("binary"))
	}))
	defer server.Close()
	cfg.Updates.BinaryURL = server.URL
	cfg.Updates.BinarySHA256 = sha256Hex("other")

	manager := NewManager(cfg)
	manager.client = server.Client()
	if err := manager.Download(context.Background()); err == nil {
		t.Fatalf("expected sha256 mismatch")
	}
	if fileExists(cfg.Kobold.BinaryPath) {
		t.Fatalf("mismatched binary should not be installed")
	}
}

func testConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Defaults()
	cfg.Kobold.BinaryPath = filepath.Join(dir, "bin", "kobold", "koboldcpp")
	cfg.Kobold.DataDir = filepath.Join(dir, "data")
	cfg.Updates.Enabled = true
	cfg.Updates.CheckInterval = 168 * time.Hour
	return cfg
}

func sha256Hex(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func sha256BytesHex(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

type archiveFile struct {
	Name    string
	Content string
}

func tarGzPayload(t *testing.T, files []archiveFile) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, file := range files {
		if err := tarWriter.WriteHeader(&tar.Header{
			Name: file.Name,
			Mode: 0o755,
			Size: int64(len(file.Content)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, file.Content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func zipPayload(t *testing.T, files []archiveFile) []byte {
	t.Helper()
	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)
	for _, file := range files {
		writer, err := zipWriter.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, file.Content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != expected {
		t.Fatalf("unexpected content for %s: %q", path, string(content))
	}
}
