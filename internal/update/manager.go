package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tensors-router/internal/config"
)

type Manager struct {
	config config.Config
	client *http.Client
	Now    func() time.Time
}

type downloadTarget struct {
	Name         string
	URL          string
	URLField     string
	SHA256       string
	SHA256Field  string
	BinaryPath   string
	DataDir      string
	MetadataName string
}

type metadata struct {
	CheckedAt    time.Time `json:"checked_at"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	URL          string    `json:"url"`
	SHA256       string    `json:"sha256"`
	BinarySHA256 string    `json:"binary_sha256,omitempty"`
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

	for _, target := range manager.targets() {
		previous := manager.readMetadata(target)
		if targetIsFresh(target, previous, manager.Now(), manager.config.Updates.CheckInterval) {
			continue
		}
		if err := manager.download(ctx, target, previous); err != nil {
			return err
		}
	}
	return nil
}

func (manager *Manager) Download(ctx context.Context) error {
	_, err := manager.DownloadedPaths(ctx)
	return err
}

func (manager *Manager) DownloadedPaths(ctx context.Context) ([]string, error) {
	paths := make([]string, 0)
	for _, target := range manager.targets() {
		if err := manager.download(ctx, target, manager.readMetadata(target)); err != nil {
			return nil, err
		}
		paths = append(paths, target.BinaryPath)
	}
	return paths, nil
}

func targetIsFresh(target downloadTarget, previous metadata, now time.Time, interval time.Duration) bool {
	if !fileExists(target.BinaryPath) ||
		previous.URL != target.URL ||
		!strings.EqualFold(previous.SHA256, target.SHA256) ||
		now.Sub(previous.CheckedAt) >= interval {
		return false
	}
	if previous.BinarySHA256 != "" {
		return fileMatchesSHA256(target.BinaryPath, previous.BinarySHA256)
	}
	return fileMatchesSHA256(target.BinaryPath, target.SHA256)
}

func (manager *Manager) targets() []downloadTarget {
	if manager.config.Backend.Mode == "llama_sdcpp" {
		return []downloadTarget{
			{
				Name:         "llama-server",
				URL:          manager.config.Updates.LlamaBinaryURL,
				URLField:     "llama_binary_url",
				SHA256:       manager.config.Updates.LlamaSHA256,
				SHA256Field:  "llama_binary_sha256",
				BinaryPath:   manager.config.Llama.BinaryPath,
				DataDir:      manager.config.Llama.DataDir,
				MetadataName: "llama-server-update.json",
			},
			{
				Name:         "sd-server",
				URL:          manager.config.Updates.SDCPPBinaryURL,
				URLField:     "sdcpp_binary_url",
				SHA256:       manager.config.Updates.SDCPPSHA256,
				SHA256Field:  "sdcpp_binary_sha256",
				BinaryPath:   manager.config.SDCPP.BinaryPath,
				DataDir:      manager.config.SDCPP.DataDir,
				MetadataName: "sd-server-update.json",
			},
		}
	}
	return []downloadTarget{
		{
			Name:         "koboldcpp",
			URL:          manager.config.Updates.BinaryURL,
			URLField:     "binary_url",
			SHA256:       manager.config.Updates.BinarySHA256,
			SHA256Field:  "binary_sha256",
			BinaryPath:   manager.config.Kobold.BinaryPath,
			DataDir:      manager.config.Kobold.DataDir,
			MetadataName: "koboldcpp-update.json",
		},
	}
}

func (manager *Manager) download(ctx context.Context, target downloadTarget, previous metadata) error {
	expectedHash, err := validateTarget(target)
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return err
	}
	canUseConditionals := previous.URL == target.URL && strings.EqualFold(previous.SHA256, target.SHA256) && fileMatchesSHA256(target.BinaryPath, target.SHA256)
	if canUseConditionals && previous.ETag != "" {
		request.Header.Set("If-None-Match", previous.ETag)
	}
	if canUseConditionals && previous.LastModified != "" {
		request.Header.Set("If-Modified-Since", previous.LastModified)
	}

	response, err := manager.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusNotModified {
		previous.CheckedAt = manager.Now()
		previous.URL = target.URL
		previous.SHA256 = normalizedSHA256(target.SHA256)
		return manager.writeMetadata(target, previous)
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("%s download failed with status %d", target.Name, response.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(target.BinaryPath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(target.DataDir, 0o755); err != nil {
		return err
	}

	tempPath := target.BinaryPath + ".download"
	output, err := os.OpenFile(tempPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)

	downloadHash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, downloadHash), response.Body)
	closeErr := output.Close()
	if copyErr != nil {
		_ = os.Remove(tempPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return closeErr
	}
	if !hashMatches(downloadHash, expectedHash) {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%s download sha256 mismatch", target.Name)
	}

	binarySHA256, err := installDownloadedPayload(tempPath, target)
	if err != nil {
		return err
	}

	next := metadata{
		CheckedAt:    manager.Now(),
		ETag:         response.Header.Get("ETag"),
		LastModified: response.Header.Get("Last-Modified"),
		URL:          target.URL,
		SHA256:       normalizedSHA256(target.SHA256),
		BinarySHA256: binarySHA256,
	}
	return manager.writeMetadata(target, next)
}

func installDownloadedPayload(payloadPath string, target downloadTarget) (string, error) {
	switch archiveType(target.URL) {
	case "zip":
		return installArchivedBinary(payloadPath, target, extractBinaryFromZip)
	case "tar.gz":
		return installArchivedBinary(payloadPath, target, extractBinaryFromTarGz)
	default:
		if err := os.Chmod(payloadPath, 0o755); err != nil {
			return "", err
		}
		if err := replaceBinary(payloadPath, target.BinaryPath); err != nil {
			return "", err
		}
		return normalizedSHA256(target.SHA256), nil
	}
}

func installArchivedBinary(payloadPath string, target downloadTarget, extract func(string, string, downloadTarget) error) (string, error) {
	extractedPath := target.BinaryPath + ".extracted"
	_ = os.Remove(extractedPath)
	if err := extract(payloadPath, extractedPath, target); err != nil {
		_ = os.Remove(extractedPath)
		return "", err
	}
	binarySHA256, err := fileSHA256Hex(extractedPath)
	if err != nil {
		_ = os.Remove(extractedPath)
		return "", err
	}
	if err := os.Chmod(extractedPath, 0o755); err != nil {
		_ = os.Remove(extractedPath)
		return "", err
	}
	if err := replaceBinary(extractedPath, target.BinaryPath); err != nil {
		_ = os.Remove(extractedPath)
		return "", err
	}
	return binarySHA256, nil
}

func archiveType(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := strings.ToLower(parsed.Path)
	switch {
	case strings.HasSuffix(path, ".zip"):
		return "zip"
	case strings.HasSuffix(path, ".tar.gz") || strings.HasSuffix(path, ".tgz"):
		return "tar.gz"
	default:
		return ""
	}
}

func extractBinaryFromZip(archivePath string, outputPath string, target downloadTarget) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !archiveEntryMatchesBinary(file.Name, target) {
			continue
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		err = writeExtractedBinary(outputPath, input)
		closeErr := input.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		return nil
	}
	return fmt.Errorf("%s archive does not contain %s", target.Name, strings.Join(binaryArchiveNames(target), " or "))
}

func extractBinaryFromTarGz(archivePath string, outputPath string, target downloadTarget) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if (header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA) || !archiveEntryMatchesBinary(header.Name, target) {
			continue
		}
		return writeExtractedBinary(outputPath, tarReader)
	}
	return fmt.Errorf("%s archive does not contain %s", target.Name, strings.Join(binaryArchiveNames(target), " or "))
}

func writeExtractedBinary(outputPath string, input io.Reader) error {
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func archiveEntryMatchesBinary(entryName string, target downloadTarget) bool {
	entryBase := filepath.Base(strings.ReplaceAll(entryName, "\\", "/"))
	for _, candidate := range binaryArchiveNames(target) {
		if strings.EqualFold(entryBase, candidate) {
			return true
		}
	}
	return false
}

func binaryArchiveNames(target downloadTarget) []string {
	values := make([]string, 0, 4)
	appendBinaryArchiveName := func(value string) {
		value = strings.TrimSpace(filepath.Base(value))
		if value == "" {
			return
		}
		for _, existing := range values {
			if strings.EqualFold(existing, value) {
				return
			}
		}
		values = append(values, value)
		if filepath.Ext(value) == "" {
			executableValue := value + ".exe"
			for _, existing := range values {
				if strings.EqualFold(existing, executableValue) {
					return
				}
			}
			values = append(values, executableValue)
		}
	}
	appendBinaryArchiveName(target.BinaryPath)
	appendBinaryArchiveName(target.Name)
	return values
}

func validateTarget(target downloadTarget) ([]byte, error) {
	if target.URL == "" {
		return nil, fmt.Errorf("updates.%s is required", target.URLField)
	}
	parsed, err := url.Parse(target.URL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("updates.%s must use https", target.URLField)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("updates.%s must include a host", target.URLField)
	}
	expectedHash, err := hex.DecodeString(strings.TrimSpace(target.SHA256))
	if err != nil || len(expectedHash) != sha256.Size {
		return nil, fmt.Errorf("updates.%s must be a 64 character SHA-256 hex digest", target.SHA256Field)
	}
	return expectedHash, nil
}

func normalizedSHA256(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func hashMatches(actual hash.Hash, expected []byte) bool {
	return bytes.Equal(actual.Sum(nil), expected)
}

func (manager *Manager) readMetadata(target downloadTarget) metadata {
	content, err := os.ReadFile(manager.metadataPath(target))
	if err != nil {
		return metadata{}
	}
	var value metadata
	if err := json.Unmarshal(content, &value); err != nil {
		return metadata{}
	}
	return value
}

func (manager *Manager) writeMetadata(target downloadTarget, value metadata) error {
	if err := os.MkdirAll(target.DataDir, 0o755); err != nil {
		return err
	}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manager.metadataPath(target), content, 0o644)
}

func (manager *Manager) metadataPath(target downloadTarget) string {
	return filepath.Join(target.DataDir, target.MetadataName)
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

func fileMatchesSHA256(path string, expected string) bool {
	expectedHash, err := hex.DecodeString(strings.TrimSpace(expected))
	if err != nil || len(expectedHash) != sha256.Size {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	actual := sha256.New()
	if _, err := io.Copy(actual, file); err != nil {
		return false
	}
	return hashMatches(actual, expectedHash)
}

func fileSHA256Hex(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	actual := sha256.New()
	if _, err := io.Copy(actual, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(actual.Sum(nil)), nil
}
