package vllm

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path"
)

//go:embed defaults
var embeddedDefaults embed.FS

// EmbeddedManifest returns the reviewed default manifest compiled into this binary for
// the given platform key, if one was shipped.
func EmbeddedManifest(platform string) ([]byte, bool) {
	if platform == "" || path.Clean(platform) != platform {
		return nil, false
	}
	content, err := fs.ReadFile(embeddedDefaults, path.Join("defaults", platform+".json"))
	if err != nil {
		return nil, false
	}
	return content, true
}

// EmbeddedManifestSource loads the default manifest compiled into this binary. It pins
// artifact bytes through the manifest itself, but carries no independent signature.
type EmbeddedManifestSource struct {
	Platform string
}

func (source EmbeddedManifestSource) Load(_ context.Context) (Manifest, string, error) {
	platform := source.Platform
	if platform == "" {
		platform = PlatformKey()
	}
	content, found := EmbeddedManifest(platform)
	if !found {
		return Manifest{}, "", fmt.Errorf("no embedded default vLLM manifest for %s: %w", platform, ErrManifestNotPublished)
	}
	manifest, err := ParseManifest(content)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("embedded default vLLM manifest for %s: %w", platform, err)
	}
	return manifest, sha256Hex(content), nil
}

func (source EmbeddedManifestSource) ManifestTrust() ManifestTrust {
	return ManifestTrustEmbeddedDefault
}
