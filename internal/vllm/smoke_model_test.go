package vllm

import (
	"archive/tar"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
		{name: "backslash traversal", header: tar.Header{Name: `..\escape`, Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, content: []byte("x"), unpacked: 1, errorMatch: "unsafe"},
		{name: "absolute", header: tar.Header{Name: "/escape", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, content: []byte("x"), unpacked: 1, errorMatch: "invalid"},
		{name: "Windows drive", header: tar.Header{Name: `C:\escape`, Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, content: []byte("x"), unpacked: 1, errorMatch: "invalid"},
		{name: "Windows UNC", header: tar.Header{Name: `\\server\share\escape`, Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}, content: []byte("x"), unpacked: 1, errorMatch: "invalid"},
		{name: "symlink", header: tar.Header{Name: "model", Linkname: "../escape", Typeflag: tar.TypeSymlink}, unpacked: 1, errorMatch: "unsupported entry"},
		{name: "hardlink", header: tar.Header{Name: "model", Linkname: "other", Typeflag: tar.TypeLink}, unpacked: 1, errorMatch: "unsupported entry"},
		{name: "device", header: tar.Header{Name: "model", Typeflag: tar.TypeChar}, unpacked: 1, errorMatch: "unsupported entry"},
		{name: "fifo", header: tar.Header{Name: "model", Typeflag: tar.TypeFifo}, unpacked: 1, errorMatch: "unsupported entry"},
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

func TestAuthorizedArchiveRejectsExistingParentSymlink(t *testing.T) {
	directory := t.TempDir()
	archivePath := filepath.Join(directory, "artifact.tar")
	writeTarArchive(t, archivePath, []tar.Header{{Name: "parent/model", Mode: 0o600, Size: 1, Typeflag: tar.TypeReg}}, [][]byte{[]byte("x")})
	destination := filepath.Join(directory, "destination")
	outside := filepath.Join(directory, "outside")
	if err := ensurePrivateDirectory(destination); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(destination, "parent")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	err := extractAuthorizedArchive(context.Background(), archivePath, destination, Artifact{ArchiveFormat: "tar", UnpackedSize: 1}, "test")
	if err == nil {
		t.Fatal("archive extraction followed an existing parent symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "model")); !os.IsNotExist(err) {
		t.Fatalf("archive escaped through parent symlink: %v", err)
	}
}

func TestArchiveRootPinsDestinationAcrossReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not permit replacing this open directory")
	}
	directory := t.TempDir()
	destination := filepath.Join(directory, "destination")
	relocated := filepath.Join(directory, "relocated")
	outside := filepath.Join(directory, "outside")
	if err := ensurePrivateDirectory(destination); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(outside); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Rename(destination, relocated); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, destination); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := writeArchiveRegularFile(context.Background(), root, "nested/model", strings.NewReader("x"), 1); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(relocated, "nested", "model"))
	if err != nil || string(content) != "x" {
		t.Fatalf("archive root did not remain pinned: content=%q error=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "nested", "model")); !os.IsNotExist(err) {
		t.Fatalf("archive escaped after destination replacement: %v", err)
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
