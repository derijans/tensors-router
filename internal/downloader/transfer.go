package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (manager *Manager) downloadFile(ctx context.Context, repository string, commit string, repositoryPath string, stagingPath string, expectedSize int64, token string) error {
	if err := ensureDirectory(filepath.Dir(stagingPath)); err != nil {
		return err
	}
	retries := manager.config.Downloads.RetryLimit
	if retries < 0 {
		retries = 0
	}
	var lastError error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * 250 * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if err := manager.downloadFileAttempt(ctx, repository, commit, repositoryPath, stagingPath, expectedSize, token); err == nil {
			return nil
		} else {
			lastError = err
			if !retryableDownloadError(err) {
				break
			}
		}
	}
	return fmt.Errorf("download %q failed: %w", repositoryPath, lastError)
}

func (manager *Manager) downloadFileAttempt(ctx context.Context, repository string, commit string, repositoryPath string, stagingPath string, expectedSize int64, token string) error {
	file, offset, err := openDownloadStaging(stagingPath, expectedSize)
	if err != nil {
		return err
	}
	defer file.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manager.hub.FileURL(repository, commit, repositoryPath), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("Accept-Encoding", "identity")
	if token = strings.TrimSpace(token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if offset > 0 {
		request.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	response, err := manager.hub.downloadClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return permanentDownloadError{message: "Hugging Face denied access; approve the gated repository and provide an authorized token"}
	}
	if response.StatusCode == http.StatusRequestedRangeNotSatisfiable && expectedSize > 0 && offset == expectedSize {
		return nil
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusRequestTimeout && response.StatusCode != http.StatusTooManyRequests {
			return permanentDownloadError{message: fmt.Sprintf("Hugging Face download returned status %d", response.StatusCode)}
		}
		return fmt.Errorf("Hugging Face download returned status %d", response.StatusCode)
	}
	if offset > 0 && response.StatusCode == http.StatusOK {
		if err := file.Truncate(0); err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		offset = 0
	}
	if response.StatusCode == http.StatusPartialContent && !strings.HasPrefix(response.Header.Get("Content-Range"), "bytes "+strconv.FormatInt(offset, 10)+"-") {
		return permanentDownloadError{message: "Hugging Face returned an invalid resume range"}
	}
	written, err := io.Copy(file, response.Body)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	actualSize := offset + written
	if expectedSize > 0 && actualSize != expectedSize {
		return fmt.Errorf("downloaded size is %d bytes, expected %d", actualSize, expectedSize)
	}
	return nil
}

func openDownloadStaging(path string, expectedSize int64) (*os.File, int64, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, 0, fmt.Errorf("download staging path is not a regular file")
		}
		if expectedSize > 0 && info.Size() > expectedSize {
			if err := os.Remove(path); err != nil {
				return nil, 0, err
			}
		} else {
			file, openErr := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
			if openErr == nil {
				openErr = file.Chmod(0o600)
			}
			return file, info.Size(), openErr
		}
	} else if !os.IsNotExist(err) {
		return nil, 0, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		err = file.Chmod(0o600)
	}
	return file, 0, err
}

type permanentDownloadError struct{ message string }

func (problem permanentDownloadError) Error() string { return problem.message }

func retryableDownloadError(err error) bool {
	var permanent permanentDownloadError
	return !errors.As(err, &permanent) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}
