package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tufmetadata "github.com/theupdateframework/go-tuf/v2/metadata"
	tufconfig "github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"

	"tensors-router/internal/hardware"
)

type signedReleaseManifest struct {
	Schema     int                     `json:"schema"`
	Repository string                  `json:"repository"`
	Releases   []signedManifestRelease `json:"releases"`
}

type signedManifestRelease struct {
	ID          string                  `json:"id"`
	Tag         string                  `json:"tag"`
	PublishedAt time.Time               `json:"published_at"`
	Prerelease  bool                    `json:"prerelease,omitempty"`
	Payloads    []signedManifestPayload `json:"payloads"`
}

type signedManifestPayload struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Length int64  `json:"length"`
	SHA256 string `json:"sha256"`
}

type preparedTrust struct {
	currentDir string
	stagingDir string
}

type contextFetcher struct {
	ctx    context.Context
	client *http.Client
}

func (fetcher contextFetcher) DownloadFile(rawURL string, maxLength int64, timeout time.Duration) ([]byte, error) {
	ctx := fetcher.ctx
	cancel := func() {}
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := fetcher.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, &tufmetadata.ErrDownloadHTTP{StatusCode: response.StatusCode, URL: rawURL}
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxLength+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maxLength {
		return nil, fmt.Errorf("trusted metadata download from %s exceeds %d bytes", rawURL, maxLength)
	}
	return content, nil
}

func (manager *Manager) prepareTrustedRelease(ctx context.Context, target downloadTarget, info hardware.Info) (resolvedRelease, *preparedTrust, error) {
	root, err := manager.trustedRoot(target)
	if err != nil {
		return resolvedRelease{}, nil, err
	}
	repositoryURL := strings.TrimRight(strings.TrimSpace(manager.config.Updates.TUFRepositoryURL), "/")
	if err := validatePayloadURL(repositoryURL, "tuf_repository_url"); err != nil {
		return resolvedRelease{}, nil, err
	}
	cacheRoot := filepath.Join(target.DataDir, "tuf", target.Name)
	if err := os.MkdirAll(cacheRoot, 0o700); err != nil {
		return resolvedRelease{}, nil, err
	}
	currentDir := filepath.Join(cacheRoot, "current")
	stagingDir, err := os.MkdirTemp(cacheRoot, ".refresh-")
	if err != nil {
		return resolvedRelease{}, nil, err
	}
	prepared := &preparedTrust{currentDir: currentDir, stagingDir: stagingDir}
	failed := true
	defer func() {
		if failed {
			prepared.Discard()
		}
	}()
	metadataDir := filepath.Join(stagingDir, "metadata")
	targetsDir := filepath.Join(stagingDir, "targets")
	if err := copyTrustedCache(filepath.Join(currentDir, "metadata"), metadataDir); err != nil {
		return resolvedRelease{}, nil, err
	}
	cachedRoot, err := os.ReadFile(filepath.Join(metadataDir, "root.json"))
	if err == nil {
		root = cachedRoot
	} else if !os.IsNotExist(err) {
		return resolvedRelease{}, nil, err
	}
	configuration, err := tufconfig.New(repositoryURL, root)
	if err != nil {
		return resolvedRelease{}, nil, err
	}
	configuration.LocalMetadataDir = metadataDir
	configuration.LocalTargetsDir = targetsDir
	configuration.RemoteTargetsURL = strings.TrimSuffix(repositoryURL, "/metadata") + "/targets"
	configuration.Fetcher = contextFetcher{ctx: ctx, client: manager.client}
	client, err := updater.New(configuration)
	if err != nil {
		return resolvedRelease{}, nil, fmt.Errorf("initialize trusted update metadata: %w", err)
	}
	if err := client.Refresh(); err != nil {
		return resolvedRelease{}, nil, fmt.Errorf("refresh trusted update metadata: %w", err)
	}
	manifestPath := path.Join("upstreams", target.Name, runtime.GOOS+"-"+runtime.GOARCH+".json")
	targetInfo, err := client.GetTargetInfo(manifestPath)
	if err != nil {
		return resolvedRelease{}, nil, fmt.Errorf("trusted manifest %s: %w", manifestPath, err)
	}
	_, content, err := client.DownloadTarget(targetInfo, filepath.Join(targetsDir, url.PathEscape(manifestPath)), "")
	if err != nil {
		return resolvedRelease{}, nil, fmt.Errorf("download trusted manifest %s: %w", manifestPath, err)
	}
	release, err := releaseFromSignedManifest(content, target, info, manager.config.Updates.IncludePrereleases)
	if err != nil {
		return resolvedRelease{}, nil, err
	}
	failed = false
	return release, prepared, nil
}

func (manager *Manager) trustedRoot(target downloadTarget) ([]byte, error) {
	rootPath := strings.TrimSpace(manager.config.Updates.TUFRootPath)
	if rootPath == "" {
		return []byte(embeddedTrustedRoot), nil
	}
	content, err := os.ReadFile(rootPath)
	if err != nil {
		return nil, fmt.Errorf("read updates.tuf_root_path: %w", err)
	}
	return content, nil
}

func releaseFromSignedManifest(content []byte, target downloadTarget, info hardware.Info, includePrereleases bool) (resolvedRelease, error) {
	var manifest signedReleaseManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return resolvedRelease{}, fmt.Errorf("decode trusted %s manifest: %w", target.Name, err)
	}
	if manifest.Schema != 1 {
		return resolvedRelease{}, fmt.Errorf("trusted %s manifest has unsupported schema %d", target.Name, manifest.Schema)
	}
	if normalizeRepositoryURL(manifest.Repository) != normalizeRepositoryURL(target.Source.RepositoryURL) {
		return resolvedRelease{}, fmt.Errorf("trusted %s manifest does not authorize repository %s", target.Name, target.Source.RepositoryURL)
	}
	releases := append([]signedManifestRelease(nil), manifest.Releases...)
	sort.SliceStable(releases, func(left int, right int) bool {
		return releases[left].PublishedAt.After(releases[right].PublishedAt)
	})
	for _, release := range releases {
		if release.Prerelease && !includePrereleases {
			continue
		}
		assets := make([]githubAsset, len(release.Payloads))
		for index, payload := range release.Payloads {
			if payload.Length <= 0 || !validManifestSHA256(payload.SHA256) {
				return resolvedRelease{}, fmt.Errorf("trusted %s manifest has invalid payload metadata for %s", target.Name, payload.Name)
			}
			if err := validatePayloadURL(payload.URL, target.URLField); err != nil {
				return resolvedRelease{}, err
			}
			assets[index] = githubAsset{Name: payload.Name, BrowserDownloadURL: payload.URL, Digest: "sha256:" + strings.ToLower(payload.SHA256), Size: payload.Length}
		}
		payloads, err := selectReleasePayloads(target.Name, assets, target.Source.AssetGlob, info)
		if err != nil {
			return resolvedRelease{}, err
		}
		return resolvedRelease{ID: release.ID, Tag: release.Tag, Payloads: payloads}, nil
	}
	return resolvedRelease{}, fmt.Errorf("trusted %s manifest has no eligible release", target.Name)
}

func normalizeRepositoryURL(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "/"))
}

func validManifestSHA256(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' && character < 'a' || character > 'f' {
			return false
		}
	}
	return true
}

func copyTrustedCache(source string, destination string) error {
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("trusted metadata cache contains unsupported entry %s", entry.Name())
		}
		content, err := os.ReadFile(filepath.Join(source, entry.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(destination, entry.Name()), content, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (prepared *preparedTrust) Discard() {
	if prepared != nil && prepared.stagingDir != "" {
		_ = os.RemoveAll(prepared.stagingDir)
	}
}
