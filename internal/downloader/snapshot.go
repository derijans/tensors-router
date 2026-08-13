package downloader

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
)

type downloadedSnapshotFile struct {
	path   string
	size   int64
	digest string
}

func computeDownloadedSnapshotDigest(storageRoot string, repository string, commit string, files []JobFile) (string, error) {
	entries := make([]downloadedSnapshotFile, 0, len(files))
	for _, file := range files {
		if err := ValidateRepositoryPath(file.Path); err != nil {
			return "", err
		}
		normalizedPath := path.Clean(file.Path)
		filePath, err := snapshotDestinationPath(storageRoot, repository, commit, normalizedPath)
		if err != nil {
			return "", err
		}
		info, err := os.Lstat(filePath)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", fmt.Errorf("snapshot file %q is not a regular file", normalizedPath)
		}
		digest, size, err := SHA256File(filePath)
		if err != nil {
			return "", err
		}
		if size != file.Size || size != info.Size() {
			return "", fmt.Errorf("snapshot file %q changed after promotion", normalizedPath)
		}
		entries = append(entries, downloadedSnapshotFile{path: normalizedPath, size: size, digest: digest})
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("snapshot contains no files")
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].path < entries[right].path })
	tree := sha256.New()
	for index, entry := range entries {
		if index > 0 && entries[index-1].path == entry.path {
			return "", fmt.Errorf("snapshot contains duplicate path %q", entry.path)
		}
		_, _ = io.WriteString(tree, entry.path)
		_, _ = tree.Write([]byte{0})
		_, _ = io.WriteString(tree, strconv.FormatInt(entry.size, 10))
		_, _ = tree.Write([]byte{0})
		_, _ = io.WriteString(tree, entry.digest)
		_, _ = tree.Write([]byte{'\n'})
	}
	return hex.EncodeToString(tree.Sum(nil)), nil
}
