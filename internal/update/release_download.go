package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (manager *Manager) downloadRelease(ctx context.Context, target downloadTarget, previous metadata) error {
	resolver := newReleaseResolver(manager.client)
	if manager.releaseAPIBase != "" {
		resolver.apiBase = manager.releaseAPIBase
	}
	release, err := resolver.resolve(ctx, target.Name, target.Source, manager.hardware.Info(ctx), manager.config.Updates.IncludePrereleases, previous)
	if err != nil {
		return err
	}
	if release.NotModified {
		previous.CheckedAt = manager.Now()
		return manager.writeMetadata(target, previous)
	}
	for index := range release.Payloads {
		if strings.TrimSpace(release.Payloads[index].SHA256) == "" && index == 0 {
			release.Payloads[index].SHA256 = target.Source.SHA256
		}
	}
	payloadPaths, payloadHashes, cleanup, err := manager.downloadReleasePayloads(ctx, target, release.Payloads)
	if err != nil {
		return err
	}
	defer cleanup()
	binaryHash, err := installReleasePayloads(payloadPaths, target)
	if err != nil {
		return err
	}
	metadataPayloads := make([]payloadMetadata, len(release.Payloads))
	for index, payload := range release.Payloads {
		metadataPayloads[index] = payloadMetadata{URL: payload.URL, SHA256: payloadHashes[index]}
	}
	primary := release.Payloads[0]
	return manager.writeMetadata(target, metadata{CheckedAt: manager.Now(), ETag: release.ETag, URL: primary.URL, SHA256: normalizedSHA256(primary.SHA256), BinarySHA256: binaryHash, ReleaseID: release.ID, ReleaseTag: release.Tag, Payloads: metadataPayloads})
}

func (manager *Manager) downloadReleasePayloads(ctx context.Context, target downloadTarget, payloads []resolvedPayload) ([]string, []string, func(), error) {
	if len(payloads) == 0 {
		return nil, nil, func() {}, fmt.Errorf("%s release has no payloads", target.Name)
	}
	if err := os.MkdirAll(target.DataDir, 0o755); err != nil {
		return nil, nil, func() {}, err
	}
	temporaryDir, err := os.MkdirTemp(target.DataDir, target.Name+"-release-")
	if err != nil {
		return nil, nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(temporaryDir) }
	paths := make([]string, len(payloads))
	hashes := make([]string, len(payloads))
	for index, payload := range payloads {
		if err := validatePayloadURL(payload.URL, target.URLField); err != nil {
			cleanup()
			return nil, nil, func() {}, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, payload.URL, nil)
		if err != nil {
			cleanup()
			return nil, nil, func() {}, err
		}
		response, err := manager.client.Do(request)
		if err != nil {
			cleanup()
			return nil, nil, func() {}, err
		}
		if response.StatusCode < 200 || response.StatusCode > 299 {
			response.Body.Close()
			cleanup()
			return nil, nil, func() {}, fmt.Errorf("%s download failed with status %d", target.Name, response.StatusCode)
		}
		if strings.TrimSpace(payload.SHA256) == "" {
			log.Printf("SECURITY WARNING: %s download from %s has no publisher or configured SHA-256; continuing is not secure against source compromise or tampering. Configure updates.%s.", target.Name, payload.URL, target.SHA256Field)
		}
		payloadPath := filepath.Join(temporaryDir, fmt.Sprintf("%d-%s", index, filepath.Base(urlPath(payload.URL))))
		output, err := os.OpenFile(payloadPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			response.Body.Close()
			cleanup()
			return nil, nil, func() {}, err
		}
		downloadHash := sha256.New()
		_, copyErr := io.Copy(io.MultiWriter(output, downloadHash), response.Body)
		closeOutputErr := output.Close()
		closeResponseErr := response.Body.Close()
		if copyErr != nil {
			cleanup()
			return nil, nil, func() {}, copyErr
		}
		if closeOutputErr != nil {
			cleanup()
			return nil, nil, func() {}, closeOutputErr
		}
		if closeResponseErr != nil {
			cleanup()
			return nil, nil, func() {}, closeResponseErr
		}
		actual := hex.EncodeToString(downloadHash.Sum(nil))
		if strings.TrimSpace(payload.SHA256) != "" && !strings.EqualFold(actual, payload.SHA256) {
			cleanup()
			return nil, nil, func() {}, fmt.Errorf("%s download sha256 mismatch", target.Name)
		}
		paths[index] = payloadPath
		hashes[index] = actual
	}
	return paths, hashes, cleanup, nil
}

func installReleasePayloads(payloadPaths []string, target downloadTarget) (string, error) {
	if len(payloadPaths) == 0 {
		return "", fmt.Errorf("%s release has no payloads", target.Name)
	}
	if len(payloadPaths) == 1 {
		return installDownloadedPayload(payloadPaths[0], target)
	}
	findBinaryPath, extract, ok := archiveInstaller(payloadPaths[0])
	if !ok {
		return "", fmt.Errorf("%s primary payload must be an archive when companion payloads are present", target.Name)
	}
	archiveBinaryPath, err := findBinaryPath(payloadPaths[0], target)
	if err != nil {
		return "", err
	}
	installDir, err := archiveInstallDir(target, archiveBinaryPath)
	if err != nil {
		return "", err
	}
	stagingDir := installDir + ".staged"
	if err := removeInstallDir(stagingDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", err
	}
	defer os.RemoveAll(stagingDir)
	if err := extract(payloadPaths[0], stagingDir, target); err != nil {
		return "", err
	}
	installedBinaryPath, err := normalizeVersionedArchiveRoot(stagingDir, archiveBinaryPath, target)
	if err != nil {
		return "", err
	}
	for _, companionPath := range payloadPaths[1:] {
		_, companionExtract, companionOK := archiveInstaller(companionPath)
		if !companionOK {
			return "", fmt.Errorf("%s companion payload must be an archive", target.Name)
		}
		if err := companionExtract(companionPath, stagingDir, target); err != nil {
			return "", err
		}
	}
	binaryPath := filepath.Join(stagingDir, filepath.FromSlash(installedBinaryPath))
	binaryHash, err := fileSHA256Hex(binaryPath)
	if err != nil {
		return "", err
	}
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return "", err
	}
	if err := swapInstallDir(stagingDir, installDir); err != nil {
		return "", err
	}
	return binaryHash, nil
}

func archiveInstaller(payloadPath string) (func(string, downloadTarget) (string, error), func(string, string, downloadTarget) error, bool) {
	switch archiveType(payloadPath) {
	case "zip":
		return zipArchiveBinaryPath, extractZipPayload, true
	case "tar.gz":
		return tarGzArchiveBinaryPath, extractTarGzPayload, true
	default:
		return nil, nil, false
	}
}

func urlPath(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "payload"
	}
	return parsed.Path
}
