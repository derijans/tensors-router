package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadReportsProgressBeforeFileCompletes(t *testing.T) {
	progressSeen := make(chan int64, 16)
	releaseRemainder := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Length", "10")
		_, _ = writer.Write([]byte("01234"))
		writer.(http.Flusher).Flush()
		select {
		case <-releaseRemainder:
		case <-request.Context().Done():
			return
		}
		_, _ = writer.Write([]byte("56789"))
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	finished := make(chan error, 1)
	go func() {
		_, err := manager.downloadFile(context.Background(), fileDownload{
			repository:     "owner/model",
			commit:         "commit",
			repositoryPath: "model.gguf",
			stagingPath:    filepath.Join(t.TempDir(), "model.gguf"),
			expectedSize:   10,
			onProgress:     func(completed int64) { progressSeen <- completed },
		})
		finished <- err
	}()
	select {
	case completed := <-progressSeen:
		if completed != 5 {
			t.Fatalf("expected progress for the first five bytes, got %d", completed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no progress was reported while the file was still downloading")
	}
	close(releaseRemainder)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
