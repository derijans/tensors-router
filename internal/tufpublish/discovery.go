package tufpublish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

type Config struct {
	Sources []Source `json:"sources"`
}

type Source struct {
	Backend            string `json:"backend"`
	Repository         string `json:"repository"`
	Platform           string `json:"platform"`
	AssetGlob          string `json:"asset_glob"`
	IncludePrereleases bool   `json:"include_prereleases,omitempty"`
}

type release struct {
	ID          int64     `json:"id"`
	Tag         string    `json:"tag_name"`
	PublishedAt time.Time `json:"published_at"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	Assets      []asset   `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type manifest struct {
	Schema     int               `json:"schema"`
	Repository string            `json:"repository"`
	Releases   []manifestRelease `json:"releases"`
}

type manifestRelease struct {
	ID          string            `json:"id"`
	Tag         string            `json:"tag"`
	PublishedAt time.Time         `json:"published_at"`
	Prerelease  bool              `json:"prerelease,omitempty"`
	Payloads    []manifestPayload `json:"payloads"`
}

type manifestPayload struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Length int64  `json:"length"`
	SHA256 string `json:"sha256"`
}

func Discover(ctx context.Context, client *http.Client, config Config) (map[string][]byte, error) {
	if len(config.Sources) == 0 {
		return nil, fmt.Errorf("publication config has no sources")
	}
	client = httpsOnlyClient(client)
	targets := make(map[string][]byte, len(config.Sources))
	for _, source := range config.Sources {
		targetPath := path.Join("upstreams", source.Backend, source.Platform+".json")
		if source.Backend == "" || source.Platform == "" || source.AssetGlob == "" {
			return nil, fmt.Errorf("source backend, platform, and asset_glob are required")
		}
		if _, exists := targets[targetPath]; exists {
			return nil, fmt.Errorf("duplicate publication target %s", targetPath)
		}
		selectedRelease, selectedAsset, err := discoverAsset(ctx, client, source)
		if err != nil {
			return nil, err
		}
		digest, length, err := hashAsset(ctx, client, selectedAsset)
		if err != nil {
			return nil, err
		}
		body, err := json.MarshalIndent(manifest{
			Schema:     1,
			Repository: source.Repository,
			Releases: []manifestRelease{{
				ID:          fmt.Sprint(selectedRelease.ID),
				Tag:         selectedRelease.Tag,
				PublishedAt: selectedRelease.PublishedAt,
				Prerelease:  selectedRelease.Prerelease,
				Payloads: []manifestPayload{{
					Name: selectedAsset.Name, URL: selectedAsset.URL, Length: length, SHA256: digest,
				}},
			}},
		}, "", "  ")
		if err != nil {
			return nil, err
		}
		targets[targetPath] = body
	}
	return targets, nil
}

func discoverAsset(ctx context.Context, client *http.Client, source Source) (release, asset, error) {
	parts, err := githubRepositoryParts(source.Repository)
	if err != nil {
		return release{}, asset{}, err
	}
	endpoint := "https://api.github.com/repos/" + parts[0] + "/" + parts[1] + "/releases?per_page=20"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return release{}, asset{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	response, err := client.Do(request)
	if err != nil {
		return release{}, asset{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return release{}, asset{}, fmt.Errorf("release discovery failed with status %d", response.StatusCode)
	}
	var releases []release
	if err := json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(&releases); err != nil {
		return release{}, asset{}, err
	}
	for _, candidate := range releases {
		if candidate.Draft || candidate.Prerelease && !source.IncludePrereleases {
			continue
		}
		matches := make([]asset, 0, 1)
		for _, candidateAsset := range candidate.Assets {
			matched, err := path.Match(source.AssetGlob, candidateAsset.Name)
			if err != nil {
				return release{}, asset{}, err
			}
			if matched {
				matches = append(matches, candidateAsset)
			}
		}
		if len(matches) != 1 {
			return release{}, asset{}, fmt.Errorf("%s release %s asset glob %q matched %d assets", source.Backend, candidate.Tag, source.AssetGlob, len(matches))
		}
		if matches[0].Size <= 0 {
			return release{}, asset{}, fmt.Errorf("%s asset has invalid size", matches[0].Name)
		}
		return candidate, matches[0], nil
	}
	return release{}, asset{}, fmt.Errorf("%s has no eligible release", source.Repository)
}

func hashAsset(ctx context.Context, client *http.Client, selected asset) (string, int64, error) {
	assetURL, err := url.Parse(selected.URL)
	if err != nil || assetURL.Scheme != "https" || assetURL.Host == "" || assetURL.User != nil {
		return "", 0, fmt.Errorf("asset URL must use HTTPS without embedded credentials")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, selected.URL, nil)
	if err != nil {
		return "", 0, err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return "", 0, fmt.Errorf("asset download failed with status %d", response.StatusCode)
	}
	hash := sha256.New()
	length, err := io.Copy(hash, io.LimitReader(response.Body, selected.Size+1))
	if err != nil {
		return "", 0, err
	}
	if length != selected.Size {
		return "", 0, fmt.Errorf("asset size mismatch for %s", selected.Name)
	}
	return hex.EncodeToString(hash.Sum(nil)), length, nil
}

func githubRepositoryParts(rawURL string) ([]string, error) {
	repository, err := url.Parse(rawURL)
	if err != nil || repository.Scheme != "https" || !strings.EqualFold(repository.Host, "github.com") || repository.User != nil || repository.RawQuery != "" || repository.Fragment != "" || repository.RawPath != "" {
		return nil, fmt.Errorf("repository must be a canonical HTTPS github.com URL without credentials, query, or fragment")
	}
	parts := strings.Split(strings.Trim(repository.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("repository must identify owner and repository")
	}
	return parts, nil
}

func httpsOnlyClient(client *http.Client) *http.Client {
	clone := *client
	originalCheckRedirect := client.CheckRedirect
	clone.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" {
			return fmt.Errorf("redirect destination must use HTTPS")
		}
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if originalCheckRedirect != nil {
			return originalCheckRedirect(request, via)
		}
		return nil
	}
	return &clone
}
