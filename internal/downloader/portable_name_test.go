package downloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

var (
	windowsFileSystem = fileSystemRules{rejectsWindowsNames: true, caseInsensitive: true}
	linuxFileSystem   = fileSystemRules{}
)

func TestWindowsRejectsNamesItCannotStore(t *testing.T) {
	for _, path := range []string{"weights:stream.gguf", "CON.txt", "configs/aux.json", "model.", "model ", "what?.gguf", "pipe|name"} {
		if _, err := repositoryPathParts(path, windowsFileSystem); err == nil {
			t.Errorf("%q was accepted although Windows cannot store it as a regular file", path)
		}
	}
	for _, path := range []string{"model.gguf", "sub/dir.v2/model-Q4_K_M.gguf", "console.json", "雪/模型.gguf"} {
		if _, err := repositoryPathParts(path, windowsFileSystem); err != nil {
			t.Errorf("%q was rejected: %v", path, err)
		}
	}
}

func TestLinuxKeepsNamesItCanStore(t *testing.T) {
	for _, path := range []string{"weights:stream.gguf", "CON.txt", "model.", "what?.gguf"} {
		if _, err := repositoryPathParts(path, linuxFileSystem); err != nil {
			t.Errorf("%q is a valid Linux file name but was rejected: %v", path, err)
		}
	}
	if _, err := repositoryPathParts("tab\tname", linuxFileSystem); err == nil {
		t.Error("control characters must be rejected on every platform")
	}
}

func TestCaseOnlyCollisionMattersOnlyOnCaseInsensitiveFileSystems(t *testing.T) {
	paths := []string{"README.md", "readme.md"}
	if _, _, collides := windowsFileSystem.collision(paths); !collides {
		t.Error("case-only collision was not detected on a case-insensitive file system")
	}
	if _, _, collides := linuxFileSystem.collision(paths); collides {
		t.Error("case-sensitive file systems store both files, so no collision may be reported")
	}
}

func TestRepositoryListingSkipsUnstorableFilesInsteadOfFailing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"sha":"0123456789abcdef0123456789abcdef01234567","siblings":[{"rfilename":"model.gguf","size":4},{"rfilename":"../escape.txt","size":1}]}`))
	}))
	defer server.Close()
	client := NewHubClient("", 0)
	client.baseURL = server.URL
	details, err := client.Repository(context.Background(), "owner/model", "main", "")
	if err != nil {
		t.Fatalf("one unstorable sibling made the whole repository unusable: %v", err)
	}
	if len(details.Files) != 1 || len(details.Skipped) != 1 || details.Skipped[0].Path != "../escape.txt" {
		t.Fatalf("unexpected listing %#v", details)
	}
	if _, err := BuildPlan(details, nil, "snapshot", t.TempDir()); err == nil {
		t.Fatal("a snapshot missing a skipped file was reported as complete")
	}
}
