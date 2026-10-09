package tufpublish

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"

	"tensors-router/internal/vllm"
)

const maximumRuntimeManifestBytes = 4 << 20

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
		body, err := upstreamTarget(ctx, client, source)
		if err != nil {
			return nil, err
		}
		targets[targetPath] = body
	}
	if len(config.RuntimeManifests) == 0 {
		return targets, nil
	}
	runtimeTargets, err := runtimeManifestTargets(config.RuntimeManifests)
	if err != nil {
		return nil, err
	}
	for targetPath, body := range runtimeTargets {
		targets[targetPath] = body
	}
	return targets, nil
}

func upstreamTarget(ctx context.Context, client *http.Client, source Source) ([]byte, error) {
	selectedRelease, selectedAsset, err := discoverAsset(ctx, client, source)
	if err != nil {
		return nil, err
	}
	digest, length, err := hashAsset(ctx, client, selectedAsset)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(manifest{
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
}

func runtimeManifestTargets(runtimeManifests map[string]string) (map[string][]byte, error) {
	manifests := make(map[string]vllm.Manifest, len(runtimeManifests))
	bodies := make(map[string][]byte, len(runtimeManifests))
	for platform, manifestPath := range runtimeManifests {
		body, manifest, err := readRuntimeManifest(platform, manifestPath)
		if err != nil {
			return nil, err
		}
		manifests[platform] = manifest
		bodies[platform] = body
	}
	if err := vllm.ValidateReleaseProfileMatrix(manifests); err != nil {
		return nil, err
	}
	targets := make(map[string][]byte, len(bodies))
	for platform, body := range bodies {
		targets[path.Join("runtimes", "vllm", platform+".json")] = body
	}
	return targets, nil
}

func readRuntimeManifest(platform string, manifestPath string) ([]byte, vllm.Manifest, error) {
	if _, required := requiredVLLMPlatforms[platform]; !required {
		return nil, vllm.Manifest{}, fmt.Errorf("unsupported vLLM runtime manifest platform %q", platform)
	}
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, vllm.Manifest{}, err
	}
	if len(body) == 0 || len(body) > maximumRuntimeManifestBytes {
		return nil, vllm.Manifest{}, fmt.Errorf("vLLM runtime manifest %s size is invalid", platform)
	}
	manifest, err := vllm.ParseManifest(body)
	if err != nil {
		return nil, vllm.Manifest{}, fmt.Errorf("vLLM runtime manifest %s: %w", platform, err)
	}
	return body, manifest, nil
}

// Upstreams publish releases holding no build output at all: llama.cpp tags a
// marker release carrying only nightly-tag.txt alongside its real per-build
// releases. Such a release is simply not a candidate, so keep looking rather
// than failing the whole publication on it. Matching more than one asset stays
// fatal, because that means the configured glob is too loose to identify a
// single download.
func selectReleaseAsset(releases []release, source Source) (release, asset, error) {
	for _, candidate := range releases {
		if candidate.Draft || candidate.Prerelease && !source.IncludePrereleases {
			continue
		}
		matches, err := matchingReleaseAssets(candidate, source.AssetGlob)
		if err != nil {
			return release{}, asset{}, err
		}
		if len(matches) > 1 {
			return release{}, asset{}, fmt.Errorf("%s release %s asset glob %q matched %d assets", source.Backend, candidate.Tag, source.AssetGlob, len(matches))
		}
		if len(matches) == 0 {
			continue
		}
		if matches[0].Size <= 0 {
			return release{}, asset{}, fmt.Errorf("%s asset has invalid size", matches[0].Name)
		}
		return candidate, matches[0], nil
	}
	return release{}, asset{}, fmt.Errorf("%s has no release carrying an asset matching %q", source.Repository, source.AssetGlob)
}

func matchingReleaseAssets(candidate release, assetGlob string) ([]asset, error) {
	matches := make([]asset, 0, 1)
	for _, candidateAsset := range candidate.Assets {
		matched, err := path.Match(assetGlob, candidateAsset.Name)
		if err != nil {
			return nil, err
		}
		if matched {
			matches = append(matches, candidateAsset)
		}
	}
	return matches, nil
}
