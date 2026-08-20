package vllm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// UnverifiedProfileID is the fixed profile ID synthesized by UnverifiedManifestSource.
const UnverifiedProfileID = "unverified-pypi"

// UnverifiedManifestSource synthesizes a single-profile manifest that installs vLLM
// directly from PyPI, with uv resolving dependencies normally instead of installing a
// pinned artifact set. It exists because no runtime manifest has ever been published to
// TUF and no embedded default is shipped yet, so without it there is no way to install
// vLLM at all.
//
// This is not an authorization boundary: nothing here is digest-pinned, and
// ValidateManifest never accepts install_method "pypi" from parsed bytes, so no
// TUF-signed or operator-pinned manifest can reach this install path. It is only ever
// selected when an operator explicitly opts in (vllm.allow_unverified_install: true),
// and it always reports ManifestTrustUnverified so the loss of integrity verification
// is visible in state, logs, and the WebUI rather than silent.
type UnverifiedManifestSource struct {
	// VLLMVersion pins `vllm==<version>`. Empty installs whatever `uv pip install
	// vllm` resolves as latest, which is inherently unpinned even by version.
	VLLMVersion string
	// PythonVersion selects the interpreter uv provisions for the isolated venv.
	PythonVersion string
}

func (source UnverifiedManifestSource) ManifestTrust() ManifestTrust {
	return ManifestTrustUnverified
}

func (source UnverifiedManifestSource) Load(context.Context) (Manifest, string, error) {
	pythonVersion := strings.TrimSpace(source.PythonVersion)
	if pythonVersion == "" {
		pythonVersion = "3.12"
	}
	vllmVersion := strings.TrimSpace(source.VLLMVersion)
	release := vllmVersion
	if release == "" {
		release = "latest"
	}
	profile := Profile{
		ID:               UnverifiedProfileID,
		VLLMVersion:      vllmVersion,
		PythonVersion:    pythonVersion,
		InstallMethod:    "pypi",
		OperatingSystems: []string{"linux", "darwin"},
		Architectures:    []string{"amd64", "arm64"},
		Devices:          []string{"cpu", "cuda", "rocm"},
	}
	manifest := Manifest{SchemaVersion: 1, Release: release, Profiles: []Profile{profile}}
	content, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, "", fmt.Errorf("synthesize unverified vLLM manifest: %w", err)
	}
	return manifest, sha256Hex(content), nil
}
