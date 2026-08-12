package vllm

import (
	"archive/tar"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthorizedArchiveRejectsTraversalSymlinkAndSizeMismatch(t *testing.T) {
	tests := []struct {
		name       string
		header     tar.Header
		content    []byte
		unpacked   int64
		errorMatch string
	}{
		{name: "traversal", header: tar.Header{Name: "../escape", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, content: []byte("x"), unpacked: 1, errorMatch: "unsafe"},
		{name: "symlink", header: tar.Header{Name: "model", Linkname: "../escape", Typeflag: tar.TypeSymlink}, unpacked: 1, errorMatch: "unsupported entry"},
		{name: "size", header: tar.Header{Name: "model", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, content: []byte("x"), unpacked: 2, errorMatch: "does not match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			archivePath := filepath.Join(directory, "artifact.tar")
			writeTarArchive(t, archivePath, []tar.Header{test.header}, [][]byte{test.content})
			destination := filepath.Join(directory, "destination")
			if err := ensurePrivateDirectory(destination); err != nil {
				t.Fatal(err)
			}
			artifact := Artifact{ArchiveFormat: "tar", UnpackedSize: test.unpacked}
			err := extractAuthorizedArchive(context.Background(), archivePath, destination, artifact, "test")
			if err == nil || !strings.Contains(err.Error(), test.errorMatch) {
				t.Fatalf("unsafe archive was accepted: %v", err)
			}
		})
	}
}

func TestAuthorizedArchiveStopsBeforeCopyWhenCancelled(t *testing.T) {
	directory := t.TempDir()
	archivePath := filepath.Join(directory, "artifact.tar")
	writeTarArchive(t, archivePath, []tar.Header{{Name: "config.json", Mode: 0o600, Size: 2, Typeflag: tar.TypeReg}}, [][]byte{[]byte("{}")})
	destination := filepath.Join(directory, "destination")
	if err := ensurePrivateDirectory(destination); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := extractAuthorizedArchive(ctx, archivePath, destination, Artifact{ArchiveFormat: "tar", UnpackedSize: 2}, "test")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled archive extraction returned %v", err)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("cancelled archive extraction wrote %#v", entries)
	}
}

func TestServingSmokeRequiresSuccessfulHealthAndStopsChild(t *testing.T) {
	directory := t.TempDir()
	modelPath := filepath.Join(directory, smokeModelDirectoryName)
	if err := os.Mkdir(modelPath, 0o700); err != nil {
		t.Fatal(err)
	}
	launcher := &smokeRuntimeLauncher{status: 503}
	tester := CommandSmokeTester{Launcher: launcher, ServingTimeout: 300 * time.Millisecond}
	err := tester.testNativeServing(context.Background(), "python", directory, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("unhealthy smoke server was accepted: %v", err)
	}
	if len(launcher.commands) != 1 {
		t.Fatalf("smoke server was not launched exactly once: %#v", launcher.commands)
	}
}
