package vllm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAuthorizedManifestRequiresExactTUFMetadata(t *testing.T) {
	manifest := testManifest()
	content, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	authorization := ArtifactAuthorization{Length: int64(len(content)), SHA256: hex.EncodeToString(digest[:])}
	parsed, parsedDigest, err := ParseAuthorizedManifest(content, authorization)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Release != manifest.Release || parsedDigest != authorization.SHA256 {
		t.Fatalf("unexpected parsed manifest %#v digest=%q", parsed, parsedDigest)
	}
	if _, _, err := ParseAuthorizedManifest(append(content, ' '), authorization); err == nil {
		t.Fatal("expected authorized length rejection")
	}
	tampered := append([]byte(nil), content...)
	tampered[len(tampered)-2] ^= 1
	if _, _, err := ParseAuthorizedManifest(tampered, authorization); err == nil {
		t.Fatal("expected authorized digest rejection")
	}
}

func TestManifestRejectsUnpinnedAndUnsafeArtifacts(t *testing.T) {
	manifest := testManifest()
	manifest.Profiles[0].VLLMVersion = "latest"
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "exact stable") {
		t.Fatalf("unexpected version validation %v", err)
	}
	manifest = testManifest()
	manifest.Profiles[0].Artifacts[0].URL = "http://example.test/uv"
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("unexpected URL validation %v", err)
	}
}

func TestManifestRequiresImmutableOCIImageAndContainerPrerequisite(t *testing.T) {
	manifest := testManifest()
	profile := &manifest.Profiles[0]
	profile.InstallMethod = "oci"
	profile.Artifacts = []Artifact{
		{Name: "runtime.oci", URL: "https://example.test/runtime.oci", Size: 2, SHA256: strings.Repeat("a", 64), Role: "oci"},
		{Name: "smoke-model.tar", URL: "https://example.test/smoke-model.tar", Size: 2, SHA256: strings.Repeat("a", 64), Role: "smoke_model", ArchiveFormat: "tar", UnpackedSize: 2},
	}
	profile.Prerequisites = []Prerequisite{{ID: "container_engine", Description: "Docker or Podman"}}
	profile.OCIImage = "vllm:latest"
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("mutable OCI image was accepted: %v", err)
	}
	profile.OCIImage = "sha256:" + strings.Repeat("b", 64)
	if err := ValidateManifest(manifest); err != nil {
		t.Fatal(err)
	}
	profile.Prerequisites = nil
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "container_engine") {
		t.Fatalf("missing container prerequisite was accepted: %v", err)
	}
}

func TestManifestRejectsAmbiguousInstallationArtifactsAndMalformedVersions(t *testing.T) {
	manifest := testManifest()
	manifest.Profiles[0].Artifacts = append(manifest.Profiles[0].Artifacts, Artifact{Name: "source.tar", URL: "https://example.test/source.tar", Size: 2, SHA256: strings.Repeat("a", 64), Role: "source"})
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "cannot contain source") {
		t.Fatalf("ambiguous wheel profile was accepted: %v", err)
	}
	manifest = testManifest()
	manifest.Profiles[0].PythonVersion = "python-main"
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "exact stable") {
		t.Fatalf("malformed Python version was accepted: %v", err)
	}
	manifest = testManifest()
	manifest.Profiles[0].PluginVersions = map[string]string{"hardware-plugin": "1.2.3"}
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "plugin artifact") {
		t.Fatalf("unbacked plugin version was accepted: %v", err)
	}
}

func TestAutoProfileDoesNotHideBrokenAcceleratorWithCPUFallback(t *testing.T) {
	manifest := testManifest()
	manifest.Profiles = append(manifest.Profiles, Profile{
		ID: "cpu", Priority: 1, VLLMVersion: "0.10.0", PythonVersion: "3.12.8", InstallMethod: "wheel",
		OperatingSystems: []string{"linux"}, Architectures: []string{"amd64"}, Devices: []string{"cpu"}, Artifacts: manifest.Profiles[0].Artifacts,
	})
	detection := Detection{
		OS:           "linux",
		Architecture: "amd64",
		Devices:      []string{"cuda", "cpu"},
		Prerequisites: map[string]bool{
			"nvidia_driver": false,
		},
	}
	if _, err := SelectProfile(manifest, "auto", detection); err == nil || !strings.Contains(err.Error(), "CPU fallback is disabled") {
		t.Fatalf("unexpected selection result %v", err)
	}
	detection.Prerequisites["nvidia_driver"] = true
	profile, err := SelectProfile(manifest, "auto", detection)
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "cuda" {
		t.Fatalf("unexpected profile %q", profile.ID)
	}
}

func testManifest() Manifest {
	artifactDigest := sha256.Sum256([]byte("uv"))
	artifacts := []Artifact{
		{Name: "uv", URL: "https://example.test/uv", Size: 2, SHA256: hex.EncodeToString(artifactDigest[:]), Role: "uv"},
		{Name: "python.tar", URL: "https://example.test/python.tar", Size: 2, SHA256: hex.EncodeToString(artifactDigest[:]), Role: "python", ArchiveFormat: "tar", UnpackedSize: 2, ExecutablePath: "bin/python"},
		{Name: "vllm.whl", URL: "https://example.test/vllm.whl", Size: 2, SHA256: hex.EncodeToString(artifactDigest[:]), Role: "vllm"},
		{Name: "smoke-model.tar", URL: "https://example.test/smoke-model.tar", Size: 2, SHA256: hex.EncodeToString(artifactDigest[:]), Role: "smoke_model", ArchiveFormat: "tar", UnpackedSize: 2},
	}
	return Manifest{
		SchemaVersion: 1,
		Release:       "2026.08.1",
		Profiles: []Profile{{
			ID: "cuda", Priority: 100, VLLMVersion: "0.10.0", PythonVersion: "3.12.8", InstallMethod: "wheel",
			OperatingSystems: []string{"linux"}, Architectures: []string{"amd64"}, Devices: []string{"cuda"},
			Prerequisites: []Prerequisite{{ID: "nvidia_driver", Description: "compatible NVIDIA driver"}},
			Artifacts:     artifacts,
		}},
	}
}
