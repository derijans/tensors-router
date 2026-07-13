package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"

	"tensors-router/internal/config"
	"tensors-router/internal/hardware"
)

type releaseResolver struct {
	client  *http.Client
	apiBase string
	now     func() time.Time
}

type githubRelease struct {
	ID          int64         `json:"id"`
	TagName     string        `json:"tag_name"`
	PublishedAt time.Time     `json:"published_at"`
	Draft       bool          `json:"draft"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

type resolvedPayload struct {
	Name   string
	URL    string
	SHA256 string
}

type resolvedRelease struct {
	ID          string
	Tag         string
	ETag        string
	NotModified bool
	Payloads    []resolvedPayload
}

func newReleaseResolver(client *http.Client) releaseResolver {
	return releaseResolver{client: client, apiBase: "https://api.github.com", now: time.Now}
}

func (resolver releaseResolver) resolve(ctx context.Context, backend string, source config.BackendUpdateSource, info hardware.Info, includePrereleases bool, previous metadata) (resolvedRelease, error) {
	endpoint, err := githubReleasesEndpoint(resolver.apiBase, source.RepositoryURL)
	if err != nil {
		return resolvedRelease{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return resolvedRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	if previous.ReleaseID != "" && previous.ETag != "" {
		request.Header.Set("If-None-Match", previous.ETag)
	}
	response, err := resolver.client.Do(request)
	if err != nil {
		return resolvedRelease{}, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		return resolvedRelease{ID: previous.ReleaseID, Tag: previous.ReleaseTag, ETag: previous.ETag, NotModified: true}, nil
	}
	if response.StatusCode != http.StatusOK {
		return resolvedRelease{}, fmt.Errorf("%s release discovery failed with status %d", backend, response.StatusCode)
	}
	var releases []githubRelease
	if err := json.NewDecoder(response.Body).Decode(&releases); err != nil {
		return resolvedRelease{}, err
	}
	release, ok := newestRelease(releases, includePrereleases)
	if !ok {
		return resolvedRelease{}, fmt.Errorf("%s repository has no eligible release", backend)
	}
	payloads, err := selectReleasePayloads(backend, release.Assets, source.AssetGlob, info)
	if err != nil {
		return resolvedRelease{}, err
	}
	return resolvedRelease{ID: fmt.Sprint(release.ID), Tag: release.TagName, ETag: response.Header.Get("ETag"), Payloads: payloads}, nil
}

func githubReleasesEndpoint(apiBase string, repositoryURL string) (string, error) {
	repository, err := url.Parse(repositoryURL)
	if err != nil {
		return "", err
	}
	if repository.Scheme != "https" || repository.Host == "" {
		return "", fmt.Errorf("repository URL must use https")
	}
	parts := strings.Split(strings.Trim(repository.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("repository URL must identify an owner and repository")
	}
	base, err := url.Parse(apiBase)
	if err != nil {
		return "", err
	}
	if base.Scheme != "https" || base.Host == "" {
		return "", fmt.Errorf("release API must use https")
	}
	base.Path = path.Join(base.Path, "repos", parts[0], parts[1], "releases")
	query := base.Query()
	query.Set("per_page", "100")
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func newestRelease(releases []githubRelease, includePrereleases bool) (githubRelease, bool) {
	candidates := make([]githubRelease, 0, len(releases))
	for _, release := range releases {
		if release.Draft || !includePrereleases && release.Prerelease {
			continue
		}
		candidates = append(candidates, release)
	}
	sort.SliceStable(candidates, func(left int, right int) bool {
		return candidates[left].PublishedAt.After(candidates[right].PublishedAt)
	})
	if len(candidates) == 0 {
		return githubRelease{}, false
	}
	return candidates[0], true
}

func selectReleasePayloads(backend string, assets []githubAsset, assetGlob string, info hardware.Info) ([]resolvedPayload, error) {
	if strings.TrimSpace(assetGlob) != "" {
		return selectGlobbedPayload(backend, assets, assetGlob)
	}
	if backend == "llama-server" && runtime.GOOS == "linux" && info.GPUBackend == hardware.GPUBackendCUDA {
		return nil, fmt.Errorf("llama-server Linux NVIDIA releases require updates.llama_asset_glob, updates.llama_repository_url with CUDA assets, or updates.llama_binary_url")
	}
	return selectKnownPayloads(backend, assets, info)
}

func selectGlobbedPayload(backend string, assets []githubAsset, assetGlob string) ([]resolvedPayload, error) {
	matches := make([]githubAsset, 0, 1)
	for _, asset := range assets {
		matched, err := path.Match(assetGlob, asset.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid asset glob %q: %w", assetGlob, err)
		}
		if matched && isPrimaryAsset(backend, asset.Name) {
			matches = append(matches, asset)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("asset glob %q must match exactly one primary %s asset, matched %d", assetGlob, backend, len(matches))
	}
	return []resolvedPayload{payloadFromAsset(matches[0])}, nil
}

func selectKnownPayloads(backend string, assets []githubAsset, info hardware.Info) ([]resolvedPayload, error) {
	candidates := make([]githubAsset, 0)
	for _, asset := range assets {
		if isPrimaryAsset(backend, asset.Name) && matchesPlatform(asset.Name) && matchesAccelerator(asset.Name, info.GPUBackend) {
			candidates = append(candidates, asset)
		}
	}
	candidates, err := compatibleRuntimeAssets(candidates, info)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", backend, err)
	}
	if len(candidates) != 1 {
		return nil, fmt.Errorf("%s release asset selection is ambiguous or incompatible (%d matches); configure an exact asset glob", backend, len(candidates))
	}
	payloads := []resolvedPayload{payloadFromAsset(candidates[0])}
	if runtime.GOOS == "windows" && info.GPUBackend == hardware.GPUBackendCUDA && (backend == "llama-server" || backend == "sd-server") {
		companions, err := selectCUDACompanions(assets, info)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", backend, err)
		}
		payloads = append(payloads, companions...)
	}
	return payloads, nil
}

func payloadFromAsset(asset githubAsset) resolvedPayload {
	return resolvedPayload{Name: asset.Name, URL: asset.BrowserDownloadURL, SHA256: strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:")}
}

func isPrimaryAsset(backend string, name string) bool {
	lowerName := strings.ToLower(name)
	if strings.Contains(lowerName, "cudart") || strings.Contains(lowerName, "checksum") || strings.HasSuffix(lowerName, ".sha256") || strings.HasSuffix(lowerName, ".sig") {
		return false
	}
	switch backend {
	case "llama-server":
		return strings.Contains(lowerName, "llama") && (strings.HasSuffix(lowerName, ".zip") || strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz"))
	case "sd-server":
		return (strings.Contains(lowerName, "stable") || strings.HasPrefix(lowerName, "sd-")) && (strings.HasSuffix(lowerName, ".zip") || strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz"))
	default:
		return strings.Contains(lowerName, "kobold") && (strings.HasSuffix(lowerName, ".zip") || strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz") || !strings.Contains(lowerName, "."))
	}
}

func matchesPlatform(name string) bool {
	lowerName := strings.ToLower(name)
	switch runtime.GOOS {
	case "windows":
		return strings.Contains(lowerName, "win")
	case "darwin":
		return strings.Contains(lowerName, "mac") || strings.Contains(lowerName, "osx")
	default:
		return strings.Contains(lowerName, "linux") || strings.Contains(lowerName, "ubuntu")
	}
}

func matchesAccelerator(name string, backend string) bool {
	lowerName := strings.ToLower(name)
	switch backend {
	case hardware.GPUBackendCUDA:
		return strings.Contains(lowerName, "cuda") || strings.Contains(lowerName, "nvidia")
	case hardware.GPUBackendROCm:
		return strings.Contains(lowerName, "rocm") || runtime.GOOS == "windows" && strings.Contains(lowerName, "hip")
	case hardware.GPUBackendVulkan:
		return strings.Contains(lowerName, "vulkan")
	case hardware.GPUBackendMetal:
		return strings.Contains(lowerName, "metal") || strings.Contains(lowerName, "mac")
	default:
		return !strings.Contains(lowerName, "cuda") && !strings.Contains(lowerName, "nvidia") && !strings.Contains(lowerName, "rocm") && !strings.Contains(lowerName, "hip") && !strings.Contains(lowerName, "vulkan") && !strings.Contains(lowerName, "metal")
	}
}

func compatibleRuntimeAssets(assets []githubAsset, info hardware.Info) ([]githubAsset, error) {
	if len(assets) <= 1 {
		return assets, nil
	}
	version := ""
	if info.GPUBackend == hardware.GPUBackendCUDA {
		version = info.CUDAVersion
	} else if info.GPUBackend == hardware.GPUBackendROCm {
		version = info.ROCmVersion
	}
	if version == "" {
		return nil, fmt.Errorf("multiple runtime packages are available but the compatible runtime version was not detected")
	}
	needle := strings.NewReplacer(".", "", "-", "", "_", "").Replace(version)
	matches := make([]githubAsset, 0, 1)
	for _, asset := range assets {
		assetName := strings.NewReplacer(".", "", "-", "", "_", "").Replace(strings.ToLower(asset.Name))
		if strings.Contains(assetName, needle) {
			matches = append(matches, asset)
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("no exact compatible runtime package for %s", version)
	}
	return matches, nil
}

func selectCUDACompanions(assets []githubAsset, info hardware.Info) ([]resolvedPayload, error) {
	companions := make([]resolvedPayload, 0, 1)
	for _, asset := range assets {
		lowerName := strings.ToLower(asset.Name)
		if !strings.Contains(lowerName, "cudart") || !matchesPlatform(asset.Name) {
			continue
		}
		if info.CUDAVersion != "" {
			needle := strings.NewReplacer(".", "", "-", "", "_", "").Replace(info.CUDAVersion)
			candidate := strings.NewReplacer(".", "", "-", "", "_", "").Replace(lowerName)
			if !strings.Contains(candidate, needle) {
				continue
			}
		}
		companions = append(companions, payloadFromAsset(asset))
	}
	if len(companions) != 1 {
		return nil, fmt.Errorf("expected exactly one matching cudart companion, found %d", len(companions))
	}
	return companions, nil
}
