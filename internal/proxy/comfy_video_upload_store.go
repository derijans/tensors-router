package proxy

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
)

var (
	errComfyUploadNotStored  = errors.New("upload is not stored on this router")
	errComfyUploadOverBudget = errors.New("upload exceeds the remaining media budget")
)

type comfyUploadedMedia struct {
	content     []byte
	contentType string
}

func (media comfyUploadedMedia) isAudio() bool {
	return strings.HasPrefix(media.contentType, "audio/")
}

type comfyVideoUpload struct {
	path        string
	contentType string
	expiresAt   time.Time
}

func (store *comfyVideoJobStore) rememberUpload(name string, media comfyUploadedMedia) error {
	name = strings.TrimSpace(name)
	if !isPlainComfyUploadName(name) {
		return fmt.Errorf("upload name %q is not a plain file name", name)
	}
	if len(media.content) == 0 {
		return fmt.Errorf("upload content is required")
	}
	if len(media.content) > maxComfyVideoUploadBytes {
		return fmt.Errorf("uploaded media exceeded the router's %d byte size cap", int64(maxComfyVideoUploadBytes))
	}
	if err := os.MkdirAll(store.dir, 0o700); err != nil {
		return fmt.Errorf("video scratch directory %q is not usable: %w", store.dir, err)
	}
	path, err := writeUploadCopy(store.dir, media.content)
	if err != nil {
		return fmt.Errorf("could not write upload copy: %w", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.sweepLocked()
	if replaced, exists := store.uploads[name]; exists {
		store.queueRemovalLocked(replaced.path)
	}
	store.uploads[name] = comfyVideoUpload{path: path, contentType: media.contentType, expiresAt: store.now().Add(comfyVideoJobLifetime)}
	return nil
}

func writeUploadCopy(dir string, content []byte) (string, error) {
	file, err := os.CreateTemp(dir, "upload-*")
	if err != nil {
		return "", err
	}
	_, writeErr := file.Write(content)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func (store *comfyVideoJobStore) readUpload(name string, byteLimit int64) (comfyUploadedMedia, error) {
	store.mu.Lock()
	upload, ok := store.uploads[strings.TrimSpace(name)]
	store.mu.Unlock()
	if !ok {
		return comfyUploadedMedia{}, errComfyUploadNotStored
	}
	file, err := os.Open(upload.path)
	if err != nil {
		return comfyUploadedMedia{}, errComfyUploadNotStored
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, byteLimit+1))
	if err != nil {
		return comfyUploadedMedia{}, errComfyUploadNotStored
	}
	if int64(len(content)) > byteLimit {
		return comfyUploadedMedia{}, errComfyUploadOverBudget
	}
	return comfyUploadedMedia{content: content, contentType: upload.contentType}, nil
}

func isPlainComfyUploadName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\:`) && strings.IndexFunc(name, unicode.IsControl) < 0
}
