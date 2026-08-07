package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (manager *Manager) downloadRelease(ctx context.Context, target downloadTarget, previous metadata) error {
	var release resolvedRelease
	var trust *preparedTrust
	var err error
	if manager.prepareRepositoryRelease != nil {
		release, trust, err = manager.prepareRepositoryRelease(ctx, target, manager.hardware.Info(ctx))
	} else {
		release, trust, err = manager.prepareTrustedRelease(ctx, target, manager.hardware.Info(ctx))
	}
	if err != nil {
		return err
	}
	if trust == nil || strings.TrimSpace(trust.currentDir) == "" || strings.TrimSpace(trust.stagingDir) == "" {
		return fmt.Errorf("%s trusted release did not prepare metadata for promotion", target.Name)
	}
	defer trust.Discard()
	payloadPaths, payloadHashes, cleanup, err := manager.downloadReleasePayloads(ctx, target, release.Payloads)
	if err != nil {
		return err
	}
	defer cleanup()
	binaryHash, binaryPromotion, err := installReleasePayloads(payloadPaths, target)
	if err != nil {
		return err
	}
	var trustPromotion *installationPromotion
	committed := false
	defer func() {
		if !committed {
			if trustPromotion != nil {
				_ = trustPromotion.Rollback()
			}
			_ = binaryPromotion.Rollback()
		}
	}()
	if err := verifyPromotedBinary(target.BinaryPath, binaryHash); err != nil {
		return err
	}
	trustPromotion, err = promoteDirectory(trust.stagingDir, trust.currentDir)
	if err != nil {
		return err
	}
	metadataPayloads := make([]payloadMetadata, len(release.Payloads))
	for index, payload := range release.Payloads {
		metadataPayloads[index] = payloadMetadata{URL: payload.URL, SHA256: payloadHashes[index]}
	}
	primary := release.Payloads[0]
	next := metadata{CheckedAt: manager.Now(), URL: primary.URL, SHA256: normalizedSHA256(primary.SHA256), BinarySHA256: binaryHash, ReleaseID: release.ID, ReleaseTag: release.Tag, Payloads: metadataPayloads}
	if err := manager.writeMetadata(target, next); err != nil {
		return err
	}
	committed = true
	trustCleanupErr := trustPromotion.Commit()
	binaryCleanupErr := binaryPromotion.Commit()
	if trustCleanupErr != nil {
		return fmt.Errorf("remove previous trusted metadata: %w", trustCleanupErr)
	}
	if binaryCleanupErr != nil {
		return fmt.Errorf("remove previous executable: %w", binaryCleanupErr)
	}
	return nil
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
		if !validManifestSHA256(strings.ToLower(strings.TrimSpace(payload.SHA256))) || payload.Length <= 0 {
			response.Body.Close()
			cleanup()
			return nil, nil, func() {}, fmt.Errorf("%s payload %s lacks trusted size or SHA-256", target.Name, payload.Name)
		}
		if response.ContentLength >= 0 && response.ContentLength != payload.Length {
			response.Body.Close()
			cleanup()
			return nil, nil, func() {}, fmt.Errorf("%s download size mismatch", target.Name)
		}
		payloadPath := filepath.Join(temporaryDir, fmt.Sprintf("%d-%s", index, filepath.Base(urlPath(payload.URL))))
		output, err := os.OpenFile(payloadPath, os.O_CREATE|os.O_RDWR|os.O_EXCL, 0o600)
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
		info, err := os.Stat(payloadPath)
		if err != nil {
			cleanup()
			return nil, nil, func() {}, err
		}
		if info.Size() != payload.Length {
			cleanup()
			return nil, nil, func() {}, fmt.Errorf("%s download size mismatch", target.Name)
		}
		if !strings.EqualFold(actual, payload.SHA256) {
			cleanup()
			return nil, nil, func() {}, fmt.Errorf("%s download sha256 mismatch", target.Name)
		}
		paths[index] = payloadPath
		hashes[index] = actual
	}
	return paths, hashes, cleanup, nil
}

func installReleasePayloads(payloadPaths []string, target downloadTarget) (string, *installationPromotion, error) {
	if len(payloadPaths) == 0 {
		return "", nil, fmt.Errorf("%s release has no payloads", target.Name)
	}
	if len(payloadPaths) == 1 {
		return installDownloadedPayload(payloadPaths[0], target)
	}
	findBinaryPath, extract, ok := archiveInstaller(payloadPaths[0])
	if !ok {
		return "", nil, fmt.Errorf("%s primary payload must be an archive when companion payloads are present", target.Name)
	}
	archiveBinaryPath, err := findBinaryPath(payloadPaths[0], target)
	if err != nil {
		return "", nil, err
	}
	installDir, err := archiveInstallDir(target, archiveBinaryPath)
	if err != nil {
		return "", nil, err
	}
	stagingDir := installDir + ".staged"
	if err := removeInstallDir(stagingDir); err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", nil, err
	}
	defer os.RemoveAll(stagingDir)
	if err := extract(payloadPaths[0], stagingDir, target); err != nil {
		return "", nil, err
	}
	installedBinaryPath, err := normalizeVersionedArchiveRoot(stagingDir, archiveBinaryPath, target)
	if err != nil {
		return "", nil, err
	}
	for _, companionPath := range payloadPaths[1:] {
		_, companionExtract, companionOK := archiveInstaller(companionPath)
		if !companionOK {
			return "", nil, fmt.Errorf("%s companion payload must be an archive", target.Name)
		}
		if err := companionExtract(companionPath, stagingDir, target); err != nil {
			return "", nil, err
		}
	}
	binaryPath := filepath.Join(stagingDir, filepath.FromSlash(installedBinaryPath))
	binaryHash, err := fileSHA256Hex(binaryPath)
	if err != nil {
		return "", nil, err
	}
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return "", nil, err
	}
	promotion, err := promoteArchiveTree(stagingDir, installDir, installedBinaryPath)
	if err != nil {
		return "", nil, err
	}
	return binaryHash, promotion, nil
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
