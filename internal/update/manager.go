package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"tensors-router/internal/config"
)

type Manager struct {
	config config.Config
	client *http.Client
	Now    func() time.Time
}

type metadata struct {
	CheckedAt    time.Time `json:"checked_at"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	URL          string    `json:"url"`
}

func NewManager(config config.Config) *Manager {
	return &Manager{
		config: config,
		client: &http.Client{
			Timeout: 0,
		},
		Now: time.Now,
	}
}

func (manager *Manager) Ensure(ctx context.Context) error {
	if !manager.config.Updates.Enabled {
		return nil
	}

	previous := manager.readMetadata()
	if fileExists(manager.config.Kobold.BinaryPath) && previous.URL == manager.config.Updates.BinaryURL && manager.Now().Sub(previous.CheckedAt) < manager.config.Updates.CheckInterval {
		return nil
	}

	return manager.download(ctx, previous)
}

func (manager *Manager) Download(ctx context.Context) error {
	return manager.download(ctx, manager.readMetadata())
}

func (manager *Manager) download(ctx context.Context, previous metadata) error {
	if manager.config.Updates.BinaryURL == "" {
		return fmt.Errorf("updates.binary_url is required")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manager.config.Updates.BinaryURL, nil)
	if err != nil {
		return err
	}
	if previous.ETag != "" {
		request.Header.Set("If-None-Match", previous.ETag)
	}
	if previous.LastModified != "" {
		request.Header.Set("If-Modified-Since", previous.LastModified)
	}

	response, err := manager.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotModified {
		previous.CheckedAt = manager.Now()
		previous.URL = manager.config.Updates.BinaryURL
		return manager.writeMetadata(previous)
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("download failed with status %d", response.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(manager.config.Kobold.BinaryPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(manager.config.Kobold.DataDir, 0o755); err != nil {
		return err
	}

	tempPath := manager.config.Kobold.BinaryPath + ".download"
	output, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(output, response.Body)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return closeErr
	}
	if err := os.Chmod(tempPath, 0o755); err != nil {
		_ = os.Remove(tempPath)
		return err
	}

	if err := replaceBinary(tempPath, manager.config.Kobold.BinaryPath); err != nil {
		return err
	}

	next := metadata{
		CheckedAt:    manager.Now(),
		ETag:         response.Header.Get("ETag"),
		LastModified: response.Header.Get("Last-Modified"),
		URL:          manager.config.Updates.BinaryURL,
	}
	return manager.writeMetadata(next)
}

func (manager *Manager) readMetadata() metadata {
	content, err := os.ReadFile(manager.metadataPath())
	if err != nil {
		return metadata{}
	}
	var value metadata
	if err := json.Unmarshal(content, &value); err != nil {
		return metadata{}
	}
	return value
}

func (manager *Manager) writeMetadata(value metadata) error {
	if err := os.MkdirAll(manager.config.Kobold.DataDir, 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manager.metadataPath(), content, 0o644)
}

func (manager *Manager) metadataPath() string {
	return filepath.Join(manager.config.Kobold.DataDir, "koboldcpp-update.json")
}

func replaceBinary(tempPath string, targetPath string) error {
	previousPath := targetPath + ".previous"
	_ = os.Remove(previousPath)

	hadPrevious := fileExists(targetPath)
	if hadPrevious {
		if err := os.Rename(targetPath, previousPath); err != nil {
			_ = os.Remove(tempPath)
			return err
		}
	}

	if err := os.Rename(tempPath, targetPath); err != nil {
		if hadPrevious {
			_ = os.Rename(previousPath, targetPath)
		}
		_ = os.Remove(tempPath)
		return err
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
