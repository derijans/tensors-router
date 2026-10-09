package tufpublish

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"tensors-router/internal/vllm"
)

const (
	maximumEvidenceManifestBytes = 4 << 20
	maximumRunnerIdentityLength  = 256
	evidenceFutureTolerance      = 5 * time.Minute
	evidenceValidityWindow       = 14 * 24 * time.Hour
)

func LoadRuntimeEvidence(directory string, expectedCommit string, now time.Time) (map[string]string, error) {
	evidence, err := decodeRuntimeEvidence(directory)
	if err != nil {
		return nil, err
	}
	if err := validateRuntimeEvidenceEnvelope(evidence, expectedCommit, now); err != nil {
		return nil, err
	}
	manifestPaths := make(map[string]string, len(evidence.Manifests))
	for _, manifestEvidence := range evidence.Manifests {
		if _, required := requiredVLLMPlatforms[manifestEvidence.Platform]; !required {
			return nil, fmt.Errorf("unsupported vLLM evidence platform %q", manifestEvidence.Platform)
		}
		if _, duplicate := manifestPaths[manifestEvidence.Platform]; duplicate {
			return nil, fmt.Errorf("duplicate vLLM evidence platform %q", manifestEvidence.Platform)
		}
		manifestPath, err := verifiedEvidenceManifest(directory, manifestEvidence)
		if err != nil {
			return nil, err
		}
		manifestPaths[manifestEvidence.Platform] = manifestPath
	}
	return manifestPaths, nil
}

func decodeRuntimeEvidence(directory string) (RuntimeEvidence, error) {
	content, err := readBoundedEvidenceFile(filepath.Join(directory, "evidence.json"), maximumRuntimeEvidenceBytes)
	if err != nil {
		return RuntimeEvidence{}, fmt.Errorf("read vLLM runtime evidence: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var evidence RuntimeEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return RuntimeEvidence{}, fmt.Errorf("decode vLLM runtime evidence: %w", err)
	}
	if err := requireEvidenceEOF(decoder); err != nil {
		return RuntimeEvidence{}, err
	}
	return evidence, nil
}

func validateRuntimeEvidenceEnvelope(evidence RuntimeEvidence, expectedCommit string, now time.Time) error {
	if evidence.Schema != 1 {
		return fmt.Errorf("unsupported vLLM runtime evidence schema %d", evidence.Schema)
	}
	if !validCommit(expectedCommit) || !strings.EqualFold(evidence.SourceCommit, expectedCommit) {
		return fmt.Errorf("vLLM runtime evidence source commit does not match publication commit")
	}
	if evidence.GeneratedAt.After(now.Add(evidenceFutureTolerance)) || evidence.GeneratedAt.Before(now.Add(-evidenceValidityWindow)) {
		return fmt.Errorf("vLLM runtime evidence is outside its 14-day validity window")
	}
	// A bundle may cover a subset of the supported platforms. The publisher merges the
	// result with the runtime targets already published, so platforms absent from this
	// bundle keep their current signed manifest instead of being dropped.
	if len(evidence.Manifests) == 0 {
		return fmt.Errorf("vLLM runtime evidence must cover at least one supported platform")
	}
	if len(evidence.Manifests) > len(requiredVLLMPlatforms) {
		return fmt.Errorf("vLLM runtime evidence covers more platforms than are supported")
	}
	return nil
}

func verifiedEvidenceManifest(directory string, manifestEvidence RuntimeManifestEvidence) (string, error) {
	expectedName := manifestEvidence.Platform + ".json"
	if manifestEvidence.Path != expectedName {
		return "", fmt.Errorf("vLLM evidence manifest path for %s must be %s", manifestEvidence.Platform, expectedName)
	}
	manifestPath := filepath.Join(directory, "manifests", expectedName)
	body, err := readBoundedEvidenceFile(manifestPath, maximumEvidenceManifestBytes)
	if err != nil {
		return "", fmt.Errorf("read vLLM evidence manifest %s: %w", manifestEvidence.Platform, err)
	}
	digest := sha256.Sum256(body)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), manifestEvidence.SHA256) {
		return "", fmt.Errorf("vLLM evidence manifest %s SHA-256 mismatch", manifestEvidence.Platform)
	}
	manifest, err := vllm.ParseManifest(body)
	if err != nil {
		return "", fmt.Errorf("vLLM evidence manifest %s: %w", manifestEvidence.Platform, err)
	}
	if err := validateProfileEvidence(manifest, manifestEvidence.SHA256, manifestEvidence.ProfileResults); err != nil {
		return "", fmt.Errorf("vLLM evidence manifest %s: %w", manifestEvidence.Platform, err)
	}
	return manifestPath, nil
}

func validateProfileEvidence(manifest vllm.Manifest, manifestSHA256 string, results []RuntimeProfileEvidence) error {
	expected := expectedEvidenceInstallMethods(manifest)
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		key := evidenceResultKey(result.ProfileID, result.OperatingSystem, result.Architecture, result.Device)
		installMethod, found := expected[key]
		if !found {
			return fmt.Errorf("profile result %q does not match a manifest profile", key)
		}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate profile result %q", key)
		}
		seen[key] = struct{}{}
		if err := validateProfileResult(result, key, installMethod, manifestSHA256); err != nil {
			return err
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("profile evidence covers %d of %d platform/device combinations", len(seen), len(expected))
	}
	return nil
}

func expectedEvidenceInstallMethods(manifest vllm.Manifest) map[string]string {
	expected := make(map[string]string)
	for _, profile := range manifest.Profiles {
		for _, operatingSystem := range profile.OperatingSystems {
			for _, architecture := range profile.Architectures {
				for _, device := range profile.Devices {
					expected[evidenceResultKey(profile.ID, operatingSystem, architecture, device)] = profile.InstallMethod
				}
			}
		}
	}
	return expected
}

func validateProfileResult(result RuntimeProfileEvidence, key string, installMethod string, manifestSHA256 string) error {
	if err := validateResultRunner(result, key); err != nil {
		return err
	}
	if !strings.EqualFold(result.ManifestSHA256, manifestSHA256) {
		return fmt.Errorf("profile result %q does not match manifest SHA-256", key)
	}
	if !trustedRunURL(result.RunURL) {
		return fmt.Errorf("profile result %q has invalid run URL", key)
	}
	return validateResultChecks(result, key, installMethod)
}

func validateResultRunner(result RuntimeProfileEvidence, key string) error {
	if strings.TrimSpace(result.Runner) == "" || len(result.Runner) > maximumRunnerIdentityLength {
		return fmt.Errorf("profile result %q has no hardware runner identity", key)
	}
	// A GitHub-hosted runner has no accelerator, so it can only attest a CPU
	// profile. Everything else still requires a protected self-hosted runner.
	switch result.RunnerClass {
	case "self-hosted":
		return nil
	case "github-hosted":
		if result.Device != "cpu" {
			return fmt.Errorf("profile result %q was produced on a GitHub-hosted runner, which cannot validate a %s profile", key, result.Device)
		}
		return nil
	default:
		return fmt.Errorf("profile result %q has unsupported runner class %q", key, result.RunnerClass)
	}
}

func trustedRunURL(value string) bool {
	runURL, err := url.Parse(value)
	runID := strings.TrimPrefix(value, trustedProfileRunPrefix)
	return err == nil && runURL.RawQuery == "" && runURL.Fragment == "" && runURL.User == nil && runID != value && numericRunID(runID)
}

func validateResultChecks(result RuntimeProfileEvidence, key string, installMethod string) error {
	checks := []struct {
		name   string
		status string
	}{
		{"installation", result.Installation},
		{"import", result.Import},
		{"serve", result.Serve},
		{"Python audit", result.PythonDependencyAudit},
		{"runtime scan", result.RuntimeScan},
	}
	for _, check := range checks {
		if check.status != "passed" {
			return fmt.Errorf("profile result %q %s did not pass", key, check.name)
		}
	}
	if installMethod == "oci" {
		if result.ContainerScan != "passed" {
			return fmt.Errorf("profile result %q container scan did not pass", key)
		}
		return nil
	}
	if result.ContainerScan != "passed" && result.ContainerScan != "not_applicable" {
		return fmt.Errorf("profile result %q has invalid container scan status", key)
	}
	return nil
}
