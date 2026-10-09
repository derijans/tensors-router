package update

import (
	"os"
	"path/filepath"
	"testing"
)

func useInstallPathCaseRule(t *testing.T, foldCase bool) {
	t.Helper()
	previous := installPathsFoldCase
	installPathsFoldCase = foldCase
	t.Cleanup(func() { installPathsFoldCase = previous })
}

func TestArchiveInstallDirOnCaseSensitiveHostNeverReturnsTheBinaryItself(t *testing.T) {
	useInstallPathCaseRule(t, false)
	binaryPath := filepath.Join("opt", "backend", "bin", "Server")

	installDir, err := archiveInstallDir(downloadTarget{Name: "backend", BinaryPath: binaryPath}, "bin/server")
	if err != nil {
		t.Fatal(err)
	}

	if installDir != filepath.Join("opt", "backend", "bin") {
		t.Fatalf("install dir %q must be the binary's directory when the archive path differs in case", installDir)
	}
}

func TestArchiveInstallDirOnCaseInsensitiveHostTrimsTheArchivePath(t *testing.T) {
	useInstallPathCaseRule(t, true)
	binaryPath := filepath.Join("opt", "backend", "bin", "Server")

	installDir, err := archiveInstallDir(downloadTarget{Name: "backend", BinaryPath: binaryPath}, "bin/server")
	if err != nil {
		t.Fatal(err)
	}

	if installDir != filepath.Join("opt", "backend") {
		t.Fatalf("install dir %q must drop the archive path that names the same file", installDir)
	}
}

func TestArchivePromotionOnCaseSensitiveHostRemovesAFileDifferingOnlyInCase(t *testing.T) {
	if installPathsFoldCase {
		t.Skip("needs a case-sensitive filesystem")
	}
	root := t.TempDir()
	stagingDir := filepath.Join(root, "staging")
	targetDir := filepath.Join(root, "installed")
	for _, dir := range []string{stagingDir, targetDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "server"), []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "libx.so"), []byte("new library"), 0o644); err != nil {
		t.Fatal(err)
	}
	staleLibrary := filepath.Join(targetDir, "Libx.so")
	if err := os.WriteFile(staleLibrary, []byte("old library"), 0o644); err != nil {
		t.Fatal(err)
	}

	promotion, err := promoteArchiveTree(stagingDir, targetDir, "server")
	if err != nil {
		t.Fatal(err)
	}

	if !containsPath(promotion.obsolete, staleLibrary) {
		t.Fatalf("Libx.so is a different file than libx.so on a case-sensitive host and must be marked obsolete: %v", promotion.obsolete)
	}
	_ = promotion.Rollback()
}

func containsPath(paths []string, candidate string) bool {
	for _, path := range paths {
		if path == candidate {
			return true
		}
	}
	return false
}
