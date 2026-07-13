package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"tensors-router/internal/config"
	"tensors-router/internal/hardware"
)

func TestResolverSelectsStableReleaseByDefault(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/repos/owner/repository/releases" {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write([]byte(`[
{"id": 1, "tag_name": "stable", "published_at": "2026-01-01T00:00:00Z", "assets": [{"name":"llama-linux-cpu.tar.gz"}]},
{"id": 2, "tag_name": "preview", "published_at": "2026-02-01T00:00:00Z", "prerelease": true, "assets": [{"name":"llama-linux-cpu.tar.gz"}]}
]`))
	}))
	defer server.Close()
	resolver := newReleaseResolver(server.Client())
	resolver.apiBase = server.URL
	result, err := resolver.resolve(context.Background(), "llama-server", config.BackendUpdateSource{RepositoryURL: "https://github.com/owner/repository", AssetGlob: "*"}, hardware.Info{}, false, metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "1" || result.Tag != "stable" {
		t.Fatalf("unexpected release %#v", result)
	}
}

func TestResolverIncludesPrereleaseWhenConfigured(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`[
{"id": 1, "tag_name": "stable", "published_at": "2026-01-01T00:00:00Z", "assets": [{"name":"llama-linux-cpu.tar.gz"}]},
{"id": 2, "tag_name": "preview", "published_at": "2026-02-01T00:00:00Z", "prerelease": true, "assets": [{"name":"llama-linux-cpu.tar.gz"}]}
]`))
	}))
	defer server.Close()
	resolver := newReleaseResolver(server.Client())
	resolver.apiBase = server.URL
	result, err := resolver.resolve(context.Background(), "llama-server", config.BackendUpdateSource{RepositoryURL: "https://github.com/owner/repository", AssetGlob: "*"}, hardware.Info{}, true, metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if result.ID != "2" || result.Tag != "preview" {
		t.Fatalf("unexpected release %#v", result)
	}
}

func TestKnownAssetSelectionRequiresExactRuntimeWhenMultipleExist(t *testing.T) {
	platform := "linux"
	if runtime.GOOS == "windows" {
		platform = "win"
	}
	if runtime.GOOS == "darwin" {
		platform = "mac"
	}
	assets := []githubAsset{
		{Name: "llama-bin-" + platform + "-rocm-6.1-x64.tar.gz"},
		{Name: "llama-bin-" + platform + "-rocm-6.2-x64.tar.gz"},
	}
	payloads, err := selectKnownPayloads("llama-server", assets, hardware.Info{GPUBackend: hardware.GPUBackendROCm, ROCmVersion: "6.2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 1 || payloads[0].Name != assets[1].Name {
		t.Fatalf("unexpected payloads %#v", payloads)
	}
	if _, err := selectKnownPayloads("llama-server", assets, hardware.Info{GPUBackend: hardware.GPUBackendROCm}); err == nil {
		t.Fatal("expected missing runtime version to reject ambiguous assets")
	}
}

func TestDownloadRepositoryReleaseNormalizesVersionedArchiveRoot(t *testing.T) {
	cfg := testConfig(t)
	cfg.Backend.Mode = "llama_sdcpp"
	cfg.Updates.LlamaBinaryURL = ""
	cfg.Updates.LlamaRepositoryURL = "https://github.com/owner/repository"
	cfg.Updates.LlamaAssetGlob = "llama-*.tar.gz"
	cfg.Updates.SDCPPBinaryURL = "https://example.test/sdcpp"
	cfg.Updates.SDCPPSHA256 = sha256Hex("sdcpp")
	binRoot := filepath.Dir(filepath.Dir(cfg.Kobold.BinaryPath))
	cfg.Llama.BinaryPath = filepath.Join(binRoot, "llama", "llama-server")
	cfg.Llama.DataDir = filepath.Join(filepath.Dir(cfg.Kobold.DataDir), "llama")
	cfg.SDCPP.BinaryPath = filepath.Join(binRoot, "sdcpp", "sd-server")
	cfg.SDCPP.DataDir = filepath.Join(filepath.Dir(cfg.Kobold.DataDir), "sdcpp")
	archive := tarGzPayload(t, []archiveFile{{Name: "llama-b999/llama-server", Content: "llama"}, {Name: "llama-b999/libllama.so", Content: "runtime"}})
	digest := sha256.Sum256(archive)
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/owner/repository/releases":
			_, _ = writer.Write([]byte(`[{"id": 1, "tag_name": "b999", "published_at": "` + time.Now().UTC().Format(time.RFC3339) + `", "assets": [{"name":"llama-b999.tar.gz","browser_download_url":"` + server.URL + `/llama-b999.tar.gz","digest":"sha256:` + hex.EncodeToString(digest[:]) + `"}]}]`))
		case "/llama-b999.tar.gz":
			_, _ = writer.Write(archive)
		case "/sdcpp":
			_, _ = writer.Write([]byte("sdcpp"))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	manager := NewManager(cfg)
	manager.client = server.Client()
	manager.hardware = hardware.NewStatic(hardware.Info{GPUBackend: hardware.GPUBackendCPU})
	manager.releaseAPIBase = server.URL
	if err := manager.downloadRelease(context.Background(), manager.targets()[0], metadata{}); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, cfg.Llama.BinaryPath, "llama")
	assertFileContent(t, filepath.Join(filepath.Dir(cfg.Llama.BinaryPath), "libllama.so"), "runtime")
	if manager.readMetadata(manager.targets()[0]).ReleaseTag != "b999" {
		t.Fatal("release metadata was not recorded")
	}
}
