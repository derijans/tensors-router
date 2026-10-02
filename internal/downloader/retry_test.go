package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestDownloadWaitsForRetryAfterOnRateLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if requests.Add(1) == 1 {
			writer.Header().Set("Retry-After", "7")
			http.Error(writer, "slow down", http.StatusTooManyRequests)
			return
		}
		_, _ = writer.Write([]byte("model"))
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	var waited []time.Duration
	manager.retryWait = func(_ context.Context, delay time.Duration) error {
		waited = append(waited, delay)
		return nil
	}
	if _, err := manager.downloadFile(context.Background(), fileDownload{repository: "owner/model", commit: "commit", repositoryPath: "model.gguf", stagingPath: filepath.Join(t.TempDir(), "model.gguf"), expectedSize: 5}); err != nil {
		t.Fatal(err)
	}
	if len(waited) != 1 || waited[0] != 7*time.Second {
		t.Fatalf("expected one wait of the server Retry-After interval, got %v", waited)
	}
}

func TestDownloadKeepsRetryingWhileEachAttemptMakesProgress(t *testing.T) {
	content := []byte("0123456789")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		offset := 0
		if value := request.Header.Get("Range"); value != "" {
			parsed, err := strconv.Atoi(value[len("bytes=") : len(value)-1])
			if err != nil {
				http.Error(writer, "bad range", http.StatusBadRequest)
				return
			}
			offset = parsed
			writer.Header().Set("Content-Range", "bytes "+strconv.Itoa(offset)+"-9/10")
		}
		writer.Header().Set("Content-Length", strconv.Itoa(len(content)-offset))
		if offset > 0 {
			writer.WriteHeader(http.StatusPartialContent)
		}
		end := min(offset+2, len(content))
		_, _ = writer.Write(content[offset:end])
		if end < len(content) {
			writer.(http.Flusher).Flush()
			panic(http.ErrAbortHandler)
		}
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	manager.config.Downloads.RetryLimit = 1
	if _, err := manager.downloadFile(context.Background(), fileDownload{repository: "owner/model", commit: "commit", repositoryPath: "model.gguf", stagingPath: filepath.Join(t.TempDir(), "model.gguf"), expectedSize: int64(len(content))}); err != nil {
		t.Fatalf("a connection that keeps delivering bytes must not exhaust the retry budget: %v", err)
	}
}

func TestDownloadGivesUpAfterRetryBudgetWithoutProgress(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Error(writer, "down", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	manager := transferTestManager(t, server.URL+"/api")
	manager.config.Downloads.RetryLimit = 2
	if _, err := manager.downloadFile(context.Background(), fileDownload{repository: "owner/model", commit: "commit", repositoryPath: "model.gguf", stagingPath: filepath.Join(t.TempDir(), "model.gguf"), expectedSize: 5}); err == nil {
		t.Fatal("download of a permanently unavailable file succeeded")
	}
	if requests.Load() != 3 {
		t.Fatalf("expected the initial attempt plus two retries, got %d requests", requests.Load())
	}
}

func TestMetadataRequestsRetryTransientServerErrors(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if requests.Add(1) < 3 {
			http.Error(writer, "temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"sha":"0123456789abcdef0123456789abcdef01234567","siblings":[{"rfilename":"model.gguf","size":4}]}`))
	}))
	defer server.Close()
	client := NewHubClient("", 0)
	client.baseURL = server.URL
	client.wait = func(context.Context, time.Duration) error { return nil }
	details, err := client.Repository(context.Background(), "owner/model", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(details.Files) != 1 || requests.Load() != 3 {
		t.Fatalf("unexpected repository details %#v after %d requests", details, requests.Load())
	}
}

func TestParseRetryAfterAcceptsSecondsAndHTTPDates(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if delay := parseRetryAfter("12", now); delay != 12*time.Second {
		t.Fatalf("unexpected seconds delay %s", delay)
	}
	if delay := parseRetryAfter(now.Add(90*time.Second).Format(http.TimeFormat), now); delay != 90*time.Second {
		t.Fatalf("unexpected HTTP-date delay %s", delay)
	}
	if delay := parseRetryAfter("soon", now); delay != 0 {
		t.Fatalf("unparseable Retry-After produced %s", delay)
	}
}
