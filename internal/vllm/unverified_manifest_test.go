package vllm

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestUnverifiedManifestSourceProducesAnUnauthorizedPyPIProfile(t *testing.T) {
	source := UnverifiedManifestSource{VLLMVersion: "0.6.3", PythonVersion: "3.12"}
	if source.ManifestTrust() != ManifestTrustUnverified {
		t.Fatalf("unexpected trust tier %q", source.ManifestTrust())
	}
	manifest, digest, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" {
		t.Fatal("expected non-empty digest")
	}
	if len(manifest.Profiles) != 1 {
		t.Fatalf("expected exactly one synthesized profile, got %d", len(manifest.Profiles))
	}
	profile := manifest.Profiles[0]
	if profile.InstallMethod != "pypi" {
		t.Fatalf("unexpected install_method %q", profile.InstallMethod)
	}
	if profile.VLLMVersion != "0.6.3" || profile.PythonVersion != "3.12" {
		t.Fatalf("unexpected pinned versions %#v", profile)
	}
	if len(profile.Artifacts) != 0 {
		t.Fatalf("expected zero artifacts, got %d", len(profile.Artifacts))
	}
	// A manifest produced this way never goes through ParseManifest/ValidateManifest,
	// and install_method "pypi" is not one ValidateManifest accepts - so no
	// TUF-signed or operator-pinned manifest could ever smuggle this profile in.
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "unsupported installation method") {
		t.Fatalf("expected ValidateManifest to reject a pypi profile from bytes, got %v", err)
	}
}

func TestUnverifiedManifestSourceDefaultsAndStability(t *testing.T) {
	source := UnverifiedManifestSource{}
	manifest, digest, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	profile := manifest.Profiles[0]
	if profile.VLLMVersion != "" {
		t.Fatalf("expected empty (latest) vLLM version, got %q", profile.VLLMVersion)
	}
	if profile.PythonVersion == "" {
		t.Fatal("expected a default Python version")
	}
	if manifest.Release != "latest" {
		t.Fatalf("expected release %q, got %q", "latest", manifest.Release)
	}
	_, repeatDigest, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if digest != repeatDigest {
		t.Fatalf("expected a stable digest across loads, got %q then %q", digest, repeatDigest)
	}
	pinned := UnverifiedManifestSource{VLLMVersion: "0.6.3"}
	_, pinnedDigest, err := pinned.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pinnedDigest == digest {
		t.Fatal("expected a different pinned version to change the digest")
	}
}

func TestNetworkInstallEnvironmentOnlyDropsOfflineSwitches(t *testing.T) {
	directory := t.TempDir()
	isolated := isolatedInstallEnvironment(directory)
	network := networkInstallEnvironment(directory)
	isolatedSet := make(map[string]struct{}, len(isolated))
	for _, entry := range isolated {
		isolatedSet[entry] = struct{}{}
	}
	networkSet := make(map[string]struct{}, len(network))
	for _, entry := range network {
		networkSet[entry] = struct{}{}
	}
	var onlyInIsolated []string
	for entry := range isolatedSet {
		if _, present := networkSet[entry]; !present {
			onlyInIsolated = append(onlyInIsolated, entry)
		}
	}
	var onlyInNetwork []string
	for entry := range networkSet {
		if _, present := isolatedSet[entry]; !present {
			onlyInNetwork = append(onlyInNetwork, entry)
		}
	}
	if len(onlyInNetwork) != 0 {
		t.Fatalf("networkInstallEnvironment adds entries beyond isolatedInstallEnvironment: %v", onlyInNetwork)
	}
	if len(onlyInIsolated) != 2 {
		t.Fatalf("expected exactly two dropped entries (UV_OFFLINE, PIP_NO_INDEX), got %v", onlyInIsolated)
	}
	for _, entry := range onlyInIsolated {
		if entry != "UV_OFFLINE=1" && entry != "PIP_NO_INDEX=1" {
			t.Fatalf("unexpected dropped entry %q", entry)
		}
	}
}

func TestServeArgumentsPreferRunnerOverRemovedTaskFlag(t *testing.T) {
	base := VLLMModelConfig{
		Snapshot: SnapshotIdentity{Path: "/models/model", TreeDigest: strings.Repeat("a", 64)},
		Task:     "embed",
		Runner:   "pooling",
	}
	arguments, err := BuildServeArguments(base, "/tmp/vllm.sock")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(arguments, " ")
	// Current vLLM exits with "unrecognized arguments: --task" before it can serve.
	if strings.Contains(joined, "--task") {
		t.Fatalf("runner-based config must not emit the removed --task flag: %s", joined)
	}
	if !strings.Contains(joined, "--runner pooling") {
		t.Fatalf("expected --runner to select the pooling runtime: %s", joined)
	}

	legacy := base
	legacy.Runner = ""
	legacyArguments, err := BuildServeArguments(legacy, "/tmp/vllm.sock")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(legacyArguments, " "), "--task embed") {
		t.Fatalf("a config with only a task must still emit --task: %v", legacyArguments)
	}
}

func TestOCIArgumentsOnlyDropHostIdentityWhenOptedIn(t *testing.T) {
	profile := Profile{OCIImage: "sha256:" + strings.Repeat("b", 64), Devices: []string{"rocm"}}
	defaulted := ociCommandArguments("docker", profile, nil, nil, []string{"-c", "pass"}, false)
	optedIn := ociCommandArguments("docker", profile, nil, nil, []string{"-c", "pass"}, false, true)

	// Identity arguments are platform-specific (empty on Windows) and include bare
	// values such as a numeric group id, so compare argument slices rather than
	// searching the joined string, where "4" also matches "--shm-size=4g".
	identity := containerIdentityArguments("docker")
	if len(defaulted)-len(optedIn) != len(identity) {
		t.Fatalf("opting in should remove exactly the %d identity arguments: %v vs %v", len(identity), defaulted, optedIn)
	}
	expected := make([]string, 0, len(optedIn))
	for index := 0; index < len(defaulted); index++ {
		if len(identity) > 0 && index+len(identity) <= len(defaulted) && slices.Equal(defaulted[index:index+len(identity)], identity) {
			index += len(identity) - 1
			continue
		}
		expected = append(expected, defaulted[index])
	}
	if !slices.Equal(expected, optedIn) {
		t.Fatalf("opting in must differ from the default only by the identity arguments:\n want %v\n got  %v", expected, optedIn)
	}
	// The rest of the containment must survive either way.
	for _, guarantee := range []string{"--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--device=/dev/kfd"} {
		if !slices.Contains(optedIn, guarantee) {
			t.Fatalf("opting in must not weaken %q: %v", guarantee, optedIn)
		}
	}
}
