package vllm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type HTTPArtifactDownloader struct {
	Client *http.Client
}

func (downloader HTTPArtifactDownloader) Download(ctx context.Context, artifact Artifact, destination string, progress func(int64)) error {
	if err := validateArtifact(artifact); err != nil {
		return err
	}
	client := downloader.Client
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Minute,
			CheckRedirect: func(request *http.Request, previous []*http.Request) error {
				if len(previous) >= 5 {
					return fmt.Errorf("too many artifact redirects")
				}
				if request.URL.Scheme != "https" || request.URL.User != nil {
					return fmt.Errorf("artifact redirect must use credential-free HTTPS")
				}
				request.Header.Del("Authorization")
				request.Header.Del("Cookie")
				return nil
			},
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artifact.URL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("Accept-Encoding", "identity")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("artifact server returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength >= 0 && response.ContentLength != artifact.Size {
		return fmt.Errorf("artifact content length %d does not match authorized size %d", response.ContentLength, artifact.Size)
	}
	if err := ensurePrivateDirectory(filepath.Dir(destination)); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	removeOnFailure := true
	defer func() {
		_ = file.Close()
		if removeOnFailure {
			_ = os.Remove(destination)
		}
	}()
	hash := sha256.New()
	written, err := copyArtifact(io.MultiWriter(file, hash), io.LimitReader(response.Body, artifact.Size+1), progress)
	if err != nil {
		return err
	}
	if written != artifact.Size {
		return fmt.Errorf("artifact size %d does not match authorized size %d", written, artifact.Size)
	}
	if !equalSHA256(hex.EncodeToString(hash.Sum(nil)), artifact.SHA256) {
		return fmt.Errorf("artifact SHA-256 does not match authorized digest")
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	removeOnFailure = false
	return nil
}

func copyArtifact(destination io.Writer, source io.Reader, progress func(int64)) (int64, error) {
	buffer := make([]byte, 256<<10)
	var total int64
	for {
		count, readError := source.Read(buffer)
		if count > 0 {
			written, writeError := destination.Write(buffer[:count])
			total += int64(written)
			if progress != nil {
				progress(total)
			}
			if writeError != nil {
				return total, writeError
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if readError != nil {
			if errors.Is(readError, io.EOF) {
				return total, nil
			}
			return total, readError
		}
	}
}

func VerifyArtifactFile(path string, artifact Artifact) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("artifact %q is not a regular file", artifact.Name)
	}
	if info.Size() != artifact.Size {
		return fmt.Errorf("artifact %q size %d does not match authorized size %d", artifact.Name, info.Size(), artifact.Size)
	}
	digest, _, err := hashStableFile(path, info)
	if err != nil {
		return err
	}
	if !equalSHA256(digest, artifact.SHA256) {
		return fmt.Errorf("artifact %q SHA-256 does not match authorized digest", artifact.Name)
	}
	return nil
}
