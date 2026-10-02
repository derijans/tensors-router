package downloader

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"strconv"
)

type stagedDigests struct {
	sha256      string
	gitBlobSHA1 string
	size        int64
}

type stagedHasher struct {
	content hash.Hash
	gitBlob hash.Hash
}

func newStagedHasher(path string, prefixLength int64, expectedSize int64) (*stagedHasher, error) {
	hasher := &stagedHasher{content: sha256.New()}
	if expectedSize >= 0 {
		hasher.gitBlob = sha1.New()
		_, _ = io.WriteString(hasher.gitBlob, "blob "+strconv.FormatInt(expectedSize, 10)+"\x00")
	}
	if prefixLength == 0 {
		return hasher, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	copied, err := io.Copy(hasher, io.LimitReader(file, prefixLength))
	if err != nil {
		return nil, err
	}
	if copied != prefixLength {
		return nil, fmt.Errorf("staged download shrank while resuming")
	}
	return hasher, nil
}

func (hasher *stagedHasher) Write(content []byte) (int, error) {
	_, _ = hasher.content.Write(content)
	if hasher.gitBlob != nil {
		_, _ = hasher.gitBlob.Write(content)
	}
	return len(content), nil
}

func (hasher *stagedHasher) digests(size int64) stagedDigests {
	result := stagedDigests{sha256: hex.EncodeToString(hasher.content.Sum(nil)), size: size}
	if hasher.gitBlob != nil {
		result.gitBlobSHA1 = hex.EncodeToString(hasher.gitBlob.Sum(nil))
	}
	return result
}

func hashStagedFile(path string, expectedSize int64) (stagedDigests, error) {
	size := stagedSize(path)
	hasher, err := newStagedHasher(path, size, expectedSize)
	if err != nil {
		return stagedDigests{}, err
	}
	return hasher.digests(size), nil
}
