package vllm

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type SnapshotFile struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type SnapshotDigest struct {
	TreeSHA256 string         `json:"tree_sha256"`
	Files      []SnapshotFile `json:"files"`
}

func ComputeSnapshotDigest(snapshotPath string) (SnapshotDigest, error) {
	rootPath, err := filepath.Abs(snapshotPath)
	if err != nil {
		return SnapshotDigest{}, err
	}
	rootInfo, err := os.Lstat(rootPath)
	if err != nil {
		return SnapshotDigest{}, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return SnapshotDigest{}, fmt.Errorf("snapshot root must be a real directory")
	}

	files := make([]SnapshotFile, 0)
	err = filepath.WalkDir(rootPath, func(candidatePath string, entry os.DirEntry, walkError error) error {
		if walkError != nil {
			return walkError
		}
		if candidatePath == rootPath {
			return nil
		}
		info, err := os.Lstat(candidatePath)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot contains symlink %q", candidatePath)
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("snapshot contains non-regular file %q", candidatePath)
		}
		relativePath, err := filepath.Rel(rootPath, candidatePath)
		if err != nil {
			return err
		}
		normalizedPath, err := normalizeSnapshotPath(relativePath)
		if err != nil {
			return err
		}
		digest, size, err := hashStableFile(candidatePath, info)
		if err != nil {
			return fmt.Errorf("hash snapshot file %q: %w", normalizedPath, err)
		}
		files = append(files, SnapshotFile{Path: normalizedPath, Size: size, SHA256: digest})
		return nil
	})
	if err != nil {
		return SnapshotDigest{}, err
	}
	if len(files) == 0 {
		return SnapshotDigest{}, fmt.Errorf("snapshot contains no files")
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	for index := 1; index < len(files); index++ {
		if files[index-1].Path == files[index].Path {
			return SnapshotDigest{}, fmt.Errorf("snapshot contains duplicate normalized path %q", files[index].Path)
		}
	}
	tree := sha256.New()
	for _, file := range files {
		_, _ = io.WriteString(tree, file.Path)
		_, _ = tree.Write([]byte{0})
		_, _ = io.WriteString(tree, strconv.FormatInt(file.Size, 10))
		_, _ = tree.Write([]byte{0})
		_, _ = io.WriteString(tree, file.SHA256)
		_, _ = tree.Write([]byte{'\n'})
	}
	return SnapshotDigest{TreeSHA256: hex.EncodeToString(tree.Sum(nil)), Files: files}, nil
}

func VerifySnapshot(identity SnapshotIdentity) error {
	if strings.TrimSpace(identity.Path) == "" || !validSHA256(identity.TreeDigest) {
		return fmt.Errorf("snapshot path and tree_sha256 are required")
	}
	digest, err := ComputeSnapshotDigest(identity.Path)
	if err != nil {
		return err
	}
	if !equalSHA256(digest.TreeSHA256, identity.TreeDigest) {
		return fmt.Errorf("snapshot tree SHA-256 does not match immutable identity")
	}
	return nil
}

func normalizeSnapshotPath(relativePath string) (string, error) {
	if relativePath == "" || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("invalid snapshot path %q", relativePath)
	}
	normalized := filepath.ToSlash(filepath.Clean(relativePath))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") || strings.ContainsRune(normalized, '\x00') {
		return "", fmt.Errorf("snapshot path escapes root: %q", relativePath)
	}
	return normalized, nil
}

func hashStableFile(path string, expected os.FileInfo) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(expected, openedInfo) {
		return "", 0, fmt.Errorf("file changed before hashing")
	}
	hash := sha256.New()
	written, err := io.Copy(hash, bufio.NewReaderSize(file, 256<<10))
	if err != nil {
		return "", 0, err
	}
	finishedInfo, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if written != openedInfo.Size() || finishedInfo.Size() != openedInfo.Size() || !finishedInfo.ModTime().Equal(openedInfo.ModTime()) {
		return "", 0, fmt.Errorf("file changed while hashing")
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}
