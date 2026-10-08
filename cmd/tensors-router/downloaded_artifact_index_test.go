package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tensors-router/internal/downloader"
	"tensors-router/internal/modelassets"
)

const recordedOnlyHash = "1111111111111111111111111111111111111111111111111111111111111111"

func TestDownloadedArtifactIndexTrustsTheDownloaderHashOnlyWhileTheFileIsUnchanged(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		source         string
		modTimeShift   int64
		wantRecordHash bool
	}{
		{name: "downloaded", source: "download", wantRecordHash: true},
		{name: "sidecar", source: "sidecar"},
		{name: "changed since download", source: "download", modTimeShift: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			index, artifact := downloadedArtifactFixture(t, testCase.source)
			artifact.ModifiedUnixNano += testCase.modTimeShift

			err := indexDownloadedArtifact(index, artifact)

			hash, _ := index.CachedFileHash(artifact.Path)
			if testCase.wantRecordHash && (err != nil || hash != recordedOnlyHash) {
				t.Fatalf("downloaded artifact was hashed again: error=%v hash=%q", err, hash)
			}
			if !testCase.wantRecordHash && (err == nil || !strings.Contains(err.Error(), "differs")) {
				t.Fatalf("unverifiable artifact was trusted: error=%v hash=%q", err, hash)
			}
		})
	}
}

func downloadedArtifactFixture(t *testing.T, source string) (*modelassets.Index, downloader.ArtifactRecord) {
	t.Helper()
	index, err := modelassets.NewIndex(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	path := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(path, []byte("weights"), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return index, downloader.ArtifactRecord{Path: path, SHA256: recordedOnlyHash, Size: info.Size(), ModifiedUnixNano: info.ModTime().UnixNano(), VerificationSource: source}
}
