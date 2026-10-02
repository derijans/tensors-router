package downloader

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestRepositoryReadsLFSDigestFromHubSHA256Field(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"sha":"0123456789abcdef0123456789abcdef01234567","siblings":[
			{"rfilename":"model.gguf","blobId":"eca68f83004d8b86fa1742ee507ddee8f4b2abce","size":1117320736,"lfs":{"sha256":"6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e","size":1117320736,"pointerSize":135}},
			{"rfilename":"README.md","blobId":"c6d9d97367083098b753d8960ac8b4a76d81039f","size":4856}]}`))
	}))
	defer server.Close()
	client := NewHubClient("", 0)
	client.baseURL = server.URL
	details, err := client.Repository(context.Background(), "owner/model", "main", "")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]File{}
	for _, file := range details.Files {
		files[file.Path] = file
	}
	if model := files["model.gguf"]; model.LFSHash != "6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e" || model.GitOID != "" {
		t.Fatalf("LFS file must verify by content SHA-256, never by the pointer blob id: %#v", model)
	}
	if readme := files["README.md"]; readme.LFSHash != "" || readme.GitOID != "c6d9d97367083098b753d8960ac8b4a76d81039f" {
		t.Fatalf("small git file must verify by its blob id: %#v", readme)
	}
}

func TestVerifyStagedFileChecksGitBlobOfSmallFiles(t *testing.T) {
	content := []byte("license text")
	blob := sha1.Sum(append([]byte("blob "+strconv.Itoa(len(content))+"\x00"), content...))
	digest := sha256.Sum256(content)
	staged := stagedDigests{sha256: hex.EncodeToString(digest[:]), gitBlobSHA1: hex.EncodeToString(blob[:]), size: int64(len(content))}
	file := JobFile{Path: "LICENSE", Size: int64(len(content)), ExpectedGitOID: hex.EncodeToString(blob[:])}
	if err := verifyStagedFile(file, staged); err != nil {
		t.Fatalf("matching git blob was rejected: %v", err)
	}
	file.ExpectedGitOID = "6634c8cc3133b3848ec74b9f275acaaa1ea618ab"
	if err := verifyStagedFile(file, staged); err == nil {
		t.Fatal("content that differs from the remote git blob was accepted")
	}
}
