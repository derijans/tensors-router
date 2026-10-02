package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadRecoversFromStalledBody(t *testing.T) {
	content := []byte("0123456789")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 1 {
			writer.Header().Set("Content-Length", strconv.Itoa(len(content)))
			_, _ = writer.Write(content[:4])
			writer.(http.Flusher).Flush()
			<-request.Context().Done()
			return
		}
		if request.Header.Get("Range") != "bytes=4-" {
			http.Error(writer, "resume must continue from the stalled offset", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Range", "bytes 4-9/10")
		writer.WriteHeader(http.StatusPartialContent)
		_, _ = writer.Write(content[4:])
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	manager.config.Downloads.StallTimeout = 200 * time.Millisecond
	stagingPath := filepath.Join(t.TempDir(), "model.gguf")
	started := time.Now()
	digests, err := manager.downloadFile(context.Background(), fileDownload{repository: "owner/model", commit: "commit", repositoryPath: "model.gguf", stagingPath: stagingPath, expectedSize: int64(len(content))})
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("stalled body was not abandoned promptly: %s", elapsed)
	}
	downloaded, err := os.ReadFile(stagingPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != string(content) || digests.size != int64(len(content)) {
		t.Fatalf("unexpected resumed content %q (%d bytes)", downloaded, digests.size)
	}
}

const proxyChildEnvironment = "TENSORS_ROUTER_DOWNLOAD_PROXY_CHILD"

func TestDownloadHonoursProxyEnvironment(t *testing.T) {
	if os.Getenv(proxyChildEnvironment) == "1" {
		downloadThroughEnvironmentProxy(t)
		return
	}
	var proxiedRequests atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Host != "huggingface.test" {
			http.Error(writer, "unexpected proxied host", http.StatusBadGateway)
			return
		}
		proxiedRequests.Add(1)
		_, _ = writer.Write([]byte("model"))
	}))
	defer proxy.Close()
	child := exec.Command(os.Args[0], "-test.run=^TestDownloadHonoursProxyEnvironment$")
	child.Env = append(os.Environ(), proxyChildEnvironment+"=1", "HTTP_PROXY="+proxy.URL, "http_proxy="+proxy.URL, "NO_PROXY=", "no_proxy=")
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("download through the environment proxy failed: %v\n%s", err, output)
	}
	if proxiedRequests.Load() == 0 {
		t.Fatal("file download bypassed the configured HTTP_PROXY")
	}
}

func downloadThroughEnvironmentProxy(t *testing.T) {
	manager := transferTestManager(t, "http://huggingface.test/api")
	stagingPath := filepath.Join(t.TempDir(), "model.gguf")
	if _, err := manager.downloadFile(context.Background(), fileDownload{repository: "owner/model", commit: "commit", repositoryPath: "model.gguf", stagingPath: stagingPath, expectedSize: 5}); err != nil {
		t.Fatal(err)
	}
}
