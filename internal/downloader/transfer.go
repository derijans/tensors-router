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
)

type fileDownload struct {
	repository     string
	commit         string
	repositoryPath string
	stagingPath    string
	expectedSize   int64
	token          string
	onProgress     func(completed int64)
}

func (manager *Manager) downloadFile(ctx context.Context, download fileDownload) (stagedDigests, error) {
	if err := ensureDirectory(filepath.Dir(download.stagingPath)); err != nil {
		return stagedDigests{}, err
	}
	retryBudget := max(manager.config.Downloads.RetryLimit, 0)
	failuresWithoutProgress := 0
	for {
		digests, written, err := manager.downloadFileAttempt(ctx, download)
		if err == nil {
			return digests, nil
		}
		if !retryableDownloadError(err) {
			return stagedDigests{}, fmt.Errorf("download %q failed: %w", download.repositoryPath, err)
		}
		if written > 0 {
			failuresWithoutProgress = 0
		}
		failuresWithoutProgress++
		if failuresWithoutProgress > retryBudget {
			return stagedDigests{}, fmt.Errorf("download %q failed after %d attempts without progress: %w", download.repositoryPath, failuresWithoutProgress, err)
		}
		manager.logRuntime("download retry path=%q attempt=%d error=%q", download.repositoryPath, failuresWithoutProgress, redactSensitive(err.Error()))
		if waitErr := manager.retryWait(ctx, backoffDelay(failuresWithoutProgress, err)); waitErr != nil {
			return stagedDigests{}, waitErr
		}
	}
}

func (manager *Manager) downloadFileAttempt(ctx context.Context, download fileDownload) (stagedDigests, int64, error) {
	file, offset, err := openDownloadStaging(download.stagingPath, download.expectedSize)
	if err != nil {
		return stagedDigests{}, 0, err
	}
	defer file.Close()
	attemptContext, guard, release := guardAgainstStall(ctx, manager.config.Downloads.StallTimeout)
	defer release()
	request, err := http.NewRequestWithContext(attemptContext, http.MethodGet, manager.hub.FileURL(download.repository, download.commit, download.repositoryPath), nil)
	if err != nil {
		return stagedDigests{}, 0, err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", userAgent())
	if token := strings.TrimSpace(download.token); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if offset > 0 {
		request.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
	}
	response, err := manager.hub.downloadClient.Do(request)
	if err != nil {
		return stagedDigests{}, 0, guard.explain(err)
	}
	defer response.Body.Close()
	if err := classifyDownloadStatus(response, download.expectedSize, offset); err != nil {
		if errors.Is(err, errAlreadyComplete) {
			digests, hashErr := hashStagedFile(download.stagingPath, download.expectedSize)
			return digests, 0, hashErr
		}
		return stagedDigests{}, 0, err
	}
	if offset > 0 && response.StatusCode == http.StatusOK {
		if err := file.Truncate(0); err != nil {
			return stagedDigests{}, 0, err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return stagedDigests{}, 0, err
		}
		offset = 0
	}
	if response.StatusCode == http.StatusPartialContent && !strings.HasPrefix(response.Header.Get("Content-Range"), "bytes "+strconv.FormatInt(offset, 10)+"-") {
		return stagedDigests{}, 0, permanentDownloadError{message: "Hugging Face returned an invalid resume range"}
	}
	hasher, err := newStagedHasher(download.stagingPath, offset, download.expectedSize)
	if err != nil {
		return stagedDigests{}, 0, err
	}
	progress := &progressCounter{completed: offset, report: download.onProgress}
	written, err := io.Copy(io.MultiWriter(file, hasher, progress), guard.reader(response.Body))
	if err != nil {
		return stagedDigests{}, written, guard.explain(err)
	}
	if err := file.Sync(); err != nil {
		return stagedDigests{}, written, err
	}
	actualSize := offset + written
	if download.expectedSize >= 0 && actualSize != download.expectedSize {
		return stagedDigests{}, written, fmt.Errorf("downloaded size is %d bytes, expected %d", actualSize, download.expectedSize)
	}
	return hasher.digests(actualSize), written, nil
}

var errAlreadyComplete = errors.New("staged file is already complete")

func classifyDownloadStatus(response *http.Response, expectedSize int64, offset int64) error {
	switch {
	case response.StatusCode == http.StatusOK || response.StatusCode == http.StatusPartialContent:
		return nil
	case response.StatusCode == http.StatusRequestedRangeNotSatisfiable && expectedSize >= 0 && offset == expectedSize:
		return errAlreadyComplete
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return permanentDownloadError{message: "Hugging Face denied access; approve the gated repository and provide an authorized token", code: ErrorDenied}
	case retryableStatus(response.StatusCode):
		return newUpstreamStatusError(response, fmt.Sprintf("Hugging Face download returned status %d", response.StatusCode))
	}
	return permanentDownloadError{message: fmt.Sprintf("Hugging Face download returned status %d", response.StatusCode), code: errorCodeForStatus(response.StatusCode)}
}

func openDownloadStaging(path string, expectedSize int64) (*os.File, int64, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, 0, fmt.Errorf("download staging path is not a regular file")
		}
		if expectedSize >= 0 && info.Size() > expectedSize {
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

func stagedSize(path string) int64 {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0
	}
	return info.Size()
}

type permanentDownloadError struct {
	message string
	code    ErrorCode
}

func (problem permanentDownloadError) Error() string { return problem.message }

func (problem permanentDownloadError) Unwrap() error {
	if problem.code == "" {
		return nil
	}
	return &Error{Code: problem.code, Message: problem.message}
}

func retryableDownloadError(err error) bool {
	var permanent permanentDownloadError
	return !errors.As(err, &permanent) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}
