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
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"tensors-router/internal/config"
	"tensors-router/internal/hardware"
)

type Manager struct {
	config         config.Config
	client         *http.Client
	hardware       hardware.Source
	releaseAPIBase string
	Now            func() time.Time
}

type downloadTarget struct {
	Name         string
	URL          string
	URLField     string
	SHA256       string
	SHA256Field  string
	Source       config.BackendUpdateSource
	BinaryPath   string
	DataDir      string
	MetadataName string
}

type metadata struct {
	CheckedAt    time.Time         `json:"checked_at"`
	ETag         string            `json:"etag,omitempty"`
	LastModified string            `json:"last_modified,omitempty"`
	URL          string            `json:"url"`
	SHA256       string            `json:"sha256"`
	BinarySHA256 string            `json:"binary_sha256,omitempty"`
	ReleaseID    string            `json:"release_id,omitempty"`
	ReleaseTag   string            `json:"release_tag,omitempty"`
	Payloads     []payloadMetadata `json:"payloads,omitempty"`
}

type payloadMetadata struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

func NewManager(config config.Config) *Manager {
	return &Manager{
		config: config,
		client: &http.Client{
			Timeout: 0,
		},
		hardware:       hardware.NewCache(),
		releaseAPIBase: "https://api.github.com",
		Now:            time.Now,
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
	if !fileExists(target.BinaryPath) || now.Sub(previous.CheckedAt) >= interval {
		return false
	}
	if target.URL == "" {
		return previous.ReleaseID != "" && previous.BinarySHA256 != "" && fileMatchesSHA256(target.BinaryPath, previous.BinarySHA256)
	}
	if previous.URL != target.URL || !strings.EqualFold(previous.SHA256, target.SHA256) {
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
				Source:       manager.config.Updates.LlamaSource(),
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
				Source:       manager.config.Updates.SDCPPSource(),
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
			Source:       manager.config.Updates.KoboldSource(),
			BinaryPath:   manager.config.Kobold.BinaryPath,
			DataDir:      manager.config.Kobold.DataDir,
			MetadataName: "koboldcpp-update.json",
		},
	}
}

func (manager *Manager) download(ctx context.Context, target downloadTarget, previous metadata) error {
	if strings.TrimSpace(target.URL) == "" {
		return manager.downloadRelease(ctx, target, previous)
	}
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
	if len(expectedHash) == 0 {
		log.Printf("SECURITY WARNING: %s download from %s has no publisher or configured SHA-256; continuing is not secure against source compromise or tampering. Configure updates.%s.", target.Name, target.URL, target.SHA256Field)
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
	if len(expectedHash) > 0 && !hashMatches(downloadHash, expectedHash) {
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
	payloadType := archiveType(target.URL)
	if payloadType == "" {
		payloadType = archiveType(payloadPath)
	}
	switch payloadType {
	case "zip":
		return installArchivedPayload(payloadPath, target, zipArchiveBinaryPath, extractZipPayload)
	case "tar.gz":
		return installArchivedPayload(payloadPath, target, tarGzArchiveBinaryPath, extractTarGzPayload)
	default:
		if err := os.Chmod(payloadPath, 0o755); err != nil {
			return "", err
		}
		if err := replaceBinary(payloadPath, target.BinaryPath); err != nil {
			return "", err
		}
		return fileSHA256Hex(target.BinaryPath)
	}
}

func installArchivedPayload(payloadPath string, target downloadTarget, findBinaryPath func(string, downloadTarget) (string, error), extract func(string, string, downloadTarget) error) (string, error) {
	archiveBinaryPath, err := findBinaryPath(payloadPath, target)
	if err != nil {
		return "", err
	}
	installDir, err := archiveInstallDir(target, archiveBinaryPath)
	if err != nil {
		return "", err
	}
	extractedDir := installDir + ".extracted"
	_ = os.RemoveAll(extractedDir)
	if err := os.MkdirAll(extractedDir, 0o755); err != nil {
		return "", err
	}
	defer os.RemoveAll(extractedDir)

	if err := extract(payloadPath, extractedDir, target); err != nil {
		return "", err
	}
	installedBinaryPath, err := normalizeVersionedArchiveRoot(extractedDir, archiveBinaryPath, target)
	if err != nil {
		return "", err
	}

	extractedBinaryPath := filepath.Join(extractedDir, filepath.FromSlash(installedBinaryPath))
	if !fileExists(extractedBinaryPath) {
		return "", fmt.Errorf("%s archive does not contain %s", target.Name, installedBinaryPath)
	}
	binarySHA256, err := fileSHA256Hex(extractedBinaryPath)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(extractedBinaryPath, 0o755); err != nil {
		return "", err
	}
	if err := swapInstallDir(extractedDir, installDir); err != nil {
		return "", err
	}
	return binarySHA256, nil
}

func archiveType(rawURL string) string {
	archivePath := rawURL
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Path != "" {
		archivePath = parsed.Path
	}
	archivePath = strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(archivePath, ".zip"):
		return "zip"
	case strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz"):
		return "tar.gz"
	default:
		return ""
	}
}

func extractZipPayload(archivePath string, outputDir string, target downloadTarget) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		info := file.FileInfo()
		if strings.Contains(file.Name, "..") {
			return fmt.Errorf("archive entry %q is not a safe relative path", file.Name)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive symlink %q is not supported", file.Name)
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			continue
		}
		outputName, ok, err := archiveEntryOutputName(file.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if info.IsDir() {
			if _, err := ensureExtractedDirectory(outputDir, outputName, 0o755); err != nil {
				return err
			}
			continue
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		err = writeExtractedFile(outputDir, outputName, input, normalizedArchiveMode(info.Mode()))
		closeErr := input.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func zipArchiveBinaryPath(archivePath string, target downloadTarget) (string, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().Mode().IsRegular() {
			names = append(names, file.Name)
		}
	}
	return archiveBinaryPathFromNames(names, target)
}

func extractTarGzPayload(archivePath string, outputDir string, target downloadTarget) error {
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
		if header.Typeflag == tar.TypeSymlink {
			return fmt.Errorf("archive symlink %q is not supported", header.Name)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA && header.Typeflag != tar.TypeDir {
			continue
		}
		if strings.Contains(header.Name, "..") {
			return fmt.Errorf("archive entry %q is not a safe relative path", header.Name)
		}
		outputName, ok, err := archiveEntryOutputName(header.Name)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if header.Typeflag == tar.TypeDir {
			if _, err := ensureExtractedDirectory(outputDir, outputName, normalizedArchiveMode(header.FileInfo().Mode())); err != nil {
				return err
			}
			continue
		}
		if err := writeExtractedFile(outputDir, outputName, tarReader, normalizedArchiveMode(header.FileInfo().Mode())); err != nil {
			return err
		}
	}
	return nil
}

func tarGzArchiveBinaryPath(archivePath string, target downloadTarget) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzipReader.Close()

	names := make([]string, 0)
	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA {
			names = append(names, header.Name)
		}
	}
	return archiveBinaryPathFromNames(names, target)
}

func ensureExtractedDirectory(root string, name string, mode os.FileMode) (string, error) {
	outputPath, err := archiveOutputPath(root, name)
	if err != nil {
		return "", err
	}
	if err := ensureExtractionRoot(root); err != nil {
		return "", err
	}
	if name == "." || name == "" {
		return outputPath, nil
	}
	currentPath := root
	for _, part := range strings.Split(name, "/") {
		currentPath = filepath.Join(currentPath, filepath.FromSlash(part))
		info, err := os.Lstat(currentPath)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("archive entry %q cannot extract through symlink %q", name, currentPath)
			}
			if !info.IsDir() {
				return "", fmt.Errorf("archive entry %q conflicts with file %q", name, currentPath)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.Mkdir(currentPath, mode); err != nil {
			return "", err
		}
	}
	if err := validateRealArchiveOutputPath(root, outputPath, name); err != nil {
		return "", err
	}
	return outputPath, nil
}

func ensureExtractionRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("archive extraction root %q is a symlink", root)
	}
	if !info.IsDir() {
		return fmt.Errorf("archive extraction root %q is not a directory", root)
	}
	return nil
}

func validateRealArchiveOutputPath(root string, outputPath string, name string) error {
	rootReal, err := resolveArchivePath(root)
	if err != nil {
		return err
	}
	outputReal, err := resolveArchivePath(outputPath)
	if err != nil {
		return err
	}
	relativePath, err := filepath.Rel(rootReal, outputReal)
	if err != nil {
		return err
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return fmt.Errorf("archive entry %q escapes extraction directory", name)
	}
	return nil
}

func resolveArchivePath(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	info, statErr := os.Lstat(path)
	if statErr == nil && info.Mode()&os.ModeSymlink == 0 {
		absolute, absErr := filepath.Abs(path)
		if absErr != nil {
			return "", absErr
		}
		return absolute, nil
	}
	return "", err
}

func writeExtractedFile(root string, name string, input io.Reader, mode os.FileMode) error {
	outputPath, err := archiveOutputPath(root, name)
	if err != nil {
		return err
	}
	if _, err := ensureExtractedDirectory(root, path.Dir(name), 0o755); err != nil {
		return err
	}
	if err := removeExtractedOutput(outputPath, name); err != nil {
		return err
	}
	output, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Chmod(outputPath, mode)
}

func removeExtractedOutput(outputPath string, name string) error {
	info, err := os.Lstat(outputPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("archive entry %q conflicts with directory %q", name, outputPath)
	}
	return os.Remove(outputPath)
}

func archiveEntryOutputName(entryName string) (string, bool, error) {
	cleanName, ok := cleanArchiveEntryName(entryName)
	if !ok {
		return "", false, fmt.Errorf("archive entry %q is not a safe relative path", entryName)
	}
	if cleanName == "" || cleanName == "." {
		return "", false, nil
	}
	return cleanName, true, nil
}

func cleanArchiveEntryName(entryName string) (string, bool) {
	cleanName := path.Clean(strings.ReplaceAll(entryName, "\\", "/"))
	if !archiveRelativePathIsSafe(cleanName) {
		return "", false
	}
	return cleanName, true
}

func archiveRelativePathIsSafe(cleanName string) bool {
	if cleanName == "" || cleanName == ".." || strings.HasPrefix(cleanName, "../") || strings.HasPrefix(cleanName, "/") {
		return false
	}
	for _, part := range strings.Split(cleanName, "/") {
		if part == "" || strings.Contains(part, ":") {
			return false
		}
	}
	return true
}

func archiveOutputPath(root string, name string) (string, error) {
	outputPath := filepath.Join(root, filepath.FromSlash(name))
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	outputAbs, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	relativePath, err := filepath.Rel(rootAbs, outputAbs)
	if err != nil {
		return "", err
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) || filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("archive entry %q escapes extraction directory", name)
	}
	return outputPath, nil
}

func archiveBinaryPathFromNames(entryNames []string, target downloadTarget) (string, error) {
	binaryNames := binaryArchiveNames(target)
	bestName := ""
	bestNameMatchesTarget := false
	bestDepth := 0
	for _, entryName := range entryNames {
		cleanName, ok := cleanArchiveEntryName(entryName)
		if !ok {
			return "", fmt.Errorf("archive entry %q is not a safe relative path", entryName)
		}
		if !matchesBinaryArchiveName(path.Base(cleanName), binaryNames) {
			continue
		}
		matchesTarget := targetPathCanContainArchivePath(target.BinaryPath, cleanName)
		depth := len(strings.Split(cleanName, "/"))
		if bestName == "" || matchesTarget && !bestNameMatchesTarget || matchesTarget == bestNameMatchesTarget && (depth < bestDepth || depth == bestDepth && cleanName < bestName) {
			bestName = cleanName
			bestNameMatchesTarget = matchesTarget
			bestDepth = depth
		}
	}
	if bestName == "" {
		return "", fmt.Errorf("%s archive does not contain %s", target.Name, strings.Join(binaryNames, " or "))
	}
	return bestName, nil
}

func targetPathCanContainArchivePath(targetPath string, archivePath string) bool {
	cleanTargetPath := filepath.ToSlash(filepath.Clean(targetPath))
	cleanArchivePath := filepath.ToSlash(filepath.Clean(archivePath))
	return strings.EqualFold(cleanTargetPath, cleanArchivePath) || strings.HasSuffix(strings.ToLower(cleanTargetPath), strings.ToLower("/"+cleanArchivePath))
}

func archiveInstallDir(target downloadTarget, archiveBinaryPath string) (string, error) {
	targetPath := filepath.ToSlash(filepath.Clean(target.BinaryPath))
	archivePath := filepath.ToSlash(filepath.Clean(archiveBinaryPath))
	suffix := "/" + archivePath
	if strings.HasSuffix(strings.ToLower(targetPath), strings.ToLower(suffix)) {
		installDir := strings.TrimSuffix(targetPath, suffix)
		if installDir == "" || installDir == "." {
			return "", fmt.Errorf("%s binary_path must include a backend install directory", target.Name)
		}
		return filepath.FromSlash(installDir), nil
	}
	installDir := filepath.Dir(target.BinaryPath)
	if installDir == "." || installDir == "" {
		return "", fmt.Errorf("%s binary_path must include a backend install directory", target.Name)
	}
	return installDir, nil
}

func normalizeVersionedArchiveRoot(extractedDir string, archiveBinaryPath string, target downloadTarget) (string, error) {
	if targetPathCanContainArchivePath(target.BinaryPath, archiveBinaryPath) {
		return archiveBinaryPath, nil
	}
	parts := strings.Split(archiveBinaryPath, "/")
	if len(parts) < 2 {
		return archiveBinaryPath, nil
	}
	entries, err := os.ReadDir(extractedDir)
	if err != nil {
		return "", err
	}
	if len(entries) != 1 || entries[0].Name() != parts[0] || !entries[0].IsDir() {
		return "", fmt.Errorf("%s archive cannot normalize its versioned top-level directory", target.Name)
	}
	root := filepath.Join(extractedDir, entries[0].Name())
	children, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	for _, child := range children {
		if err := os.Rename(filepath.Join(root, child.Name()), filepath.Join(extractedDir, child.Name())); err != nil {
			return "", err
		}
	}
	if err := os.Remove(root); err != nil {
		return "", err
	}
	return path.Join(parts[1:]...), nil
}

func swapInstallDir(extractedDir string, installDir string) error {
	if err := os.MkdirAll(filepath.Dir(installDir), 0o755); err != nil {
		return err
	}
	backupDir := installDir + ".previous"
	if err := removeInstallDir(backupDir); err != nil {
		return err
	}
	if err := os.Rename(installDir, backupDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(extractedDir, installDir); err != nil {
		if rollbackErr := os.Rename(backupDir, installDir); rollbackErr != nil && !os.IsNotExist(rollbackErr) {
			return fmt.Errorf("install failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return err
	}
	return removeInstallDir(backupDir)
}

func matchesBinaryArchiveName(entryName string, names []string) bool {
	for _, name := range names {
		if strings.EqualFold(entryName, name) {
			return true
		}
	}
	return false
}

func removeInstallDir(path string) error {
	cleanPath := filepath.Clean(path)
	if cleanPath == "." || cleanPath == "" {
		return fmt.Errorf("refusing to remove install directory %q", path)
	}
	absolutePath, err := filepath.Abs(cleanPath)
	if err != nil {
		return err
	}
	if filepath.Dir(absolutePath) == absolutePath {
		return fmt.Errorf("refusing to remove install directory %q", path)
	}
	return os.RemoveAll(cleanPath)
}

func normalizedArchiveMode(mode os.FileMode) os.FileMode {
	mode = mode.Perm()
	if mode == 0 {
		return 0o644
	}
	if mode&0o111 != 0 {
		return mode | 0o755
	}
	return mode | 0o644
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
	if err := validatePayloadURL(target.URL, target.URLField); err != nil {
		return nil, err
	}
	if strings.TrimSpace(target.SHA256) == "" {
		return nil, nil
	}
	expectedHash, err := hex.DecodeString(strings.TrimSpace(target.SHA256))
	if err != nil || len(expectedHash) != sha256.Size {
		return nil, fmt.Errorf("updates.%s must be a 64 character SHA-256 hex digest when provided", target.SHA256Field)
	}
	return expectedHash, nil
}

func validatePayloadURL(rawURL string, urlField string) error {
	if rawURL == "" {
		return fmt.Errorf("updates.%s is required", urlField)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("updates.%s must use https", urlField)
	}
	if parsed.Host == "" {
		return fmt.Errorf("updates.%s must include a host", urlField)
	}
	return nil
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
	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
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
