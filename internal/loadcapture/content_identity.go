package loadcapture

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type KnownFileHash func(absolutePath string) (string, bool)

func (builder *snapshotBuilder) contentIdentity(value string) (string, []string, error) {
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(builder.configDir, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", nil, err
	}
	variants := []string{value, absolute, filepath.Clean(absolute)}
	if info.IsDir() {
		digest, err := builder.directoryDigest(absolute)
		return digest, variants, err
	}
	digest, err := builder.fileDigest(absolute)
	return digest, variants, err
}

func (builder *snapshotBuilder) directoryDigest(path string) (string, error) {
	hashes := []string{}
	err := filepath.WalkDir(path, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if regularFileOrLinkToOne(candidate, entry) {
			digest, err := builder.fileDigest(candidate)
			if err != nil {
				return err
			}
			hashes = append(hashes, digest)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(hashes)
	sum := sha256.Sum256([]byte(strings.Join(hashes, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

func regularFileOrLinkToOne(path string, entry os.DirEntry) bool {
	if entry.Type()&os.ModeSymlink == 0 {
		return entry.Type().IsRegular()
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func (builder *snapshotBuilder) fileDigest(path string) (string, error) {
	if builder.knownFileHash != nil {
		if digest, known := builder.knownFileHash(path); known {
			return digest, nil
		}
	}
	return hashFile(path)
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
