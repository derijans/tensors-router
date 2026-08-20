package tufpublish

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"tensors-router/internal/vllm"
)

const maximumRuntimeEvidenceBytes = 1 << 20
const trustedProfileRunPrefix = "https://github.com/derijans/tensors-router/actions/runs/"

type RuntimeEvidence struct {
	Schema       int                       `json:"schema"`
	SourceCommit string                    `json:"source_commit"`
	GeneratedAt  time.Time                 `json:"generated_at"`
	Manifests    []RuntimeManifestEvidence `json:"manifests"`
}

type RuntimeManifestEvidence struct {
	Platform       string                   `json:"platform"`
	Path           string                   `json:"path"`
	SHA256         string                   `json:"sha256"`
	ProfileResults []RuntimeProfileEvidence `json:"profile_results"`
}

type RuntimeProfileEvidence struct {
	ProfileID             string `json:"profile_id"`
	OperatingSystem       string `json:"operating_system"`
	Architecture          string `json:"architecture"`
	Device                string `json:"device"`
	Runner                string `json:"runner"`
	RunnerClass           string `json:"runner_class"`
	RunURL                string `json:"run_url"`
	ManifestSHA256        string `json:"manifest_sha256"`
	Installation          string `json:"installation"`
	Import                string `json:"import"`
	Serve                 string `json:"serve"`
	PythonDependencyAudit string `json:"python_dependency_audit"`
	RuntimeScan           string `json:"runtime_vulnerability_scan"`
	ContainerScan         string `json:"container_vulnerability_scan"`
}

func LoadRuntimeEvidence(directory string, expectedCommit string, now time.Time) (map[string]string, error) {
	content, err := readBoundedEvidenceFile(filepath.Join(directory, "evidence.json"), maximumRuntimeEvidenceBytes)
	if err != nil {
		return nil, fmt.Errorf("read vLLM runtime evidence: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var evidence RuntimeEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return nil, fmt.Errorf("decode vLLM runtime evidence: %w", err)
	}
	if err := requireEvidenceEOF(decoder); err != nil {
		return nil, err
	}
	if evidence.Schema != 1 {
		return nil, fmt.Errorf("unsupported vLLM runtime evidence schema %d", evidence.Schema)
	}
	if !validCommit(expectedCommit) || !strings.EqualFold(evidence.SourceCommit, expectedCommit) {
		return nil, fmt.Errorf("vLLM runtime evidence source commit does not match publication commit")
	}
	if evidence.GeneratedAt.After(now.Add(5*time.Minute)) || evidence.GeneratedAt.Before(now.Add(-14*24*time.Hour)) {
		return nil, fmt.Errorf("vLLM runtime evidence is outside its 14-day validity window")
	}
	// A bundle may cover a subset of the supported platforms. The publisher merges the
	// result with the runtime targets already published, so platforms absent from this
	// bundle keep their current signed manifest instead of being dropped.
	if len(evidence.Manifests) == 0 {
		return nil, fmt.Errorf("vLLM runtime evidence must cover at least one supported platform")
	}
	if len(evidence.Manifests) > len(requiredVLLMPlatforms) {
		return nil, fmt.Errorf("vLLM runtime evidence covers more platforms than are supported")
	}
	manifestPaths := make(map[string]string, len(evidence.Manifests))
	for _, manifestEvidence := range evidence.Manifests {
		if _, required := requiredVLLMPlatforms[manifestEvidence.Platform]; !required {
			return nil, fmt.Errorf("unsupported vLLM evidence platform %q", manifestEvidence.Platform)
		}
		if _, duplicate := manifestPaths[manifestEvidence.Platform]; duplicate {
			return nil, fmt.Errorf("duplicate vLLM evidence platform %q", manifestEvidence.Platform)
		}
		expectedName := manifestEvidence.Platform + ".json"
		if manifestEvidence.Path != expectedName {
			return nil, fmt.Errorf("vLLM evidence manifest path for %s must be %s", manifestEvidence.Platform, expectedName)
		}
		manifestPath := filepath.Join(directory, "manifests", expectedName)
		body, err := readBoundedEvidenceFile(manifestPath, 4<<20)
		if err != nil {
			return nil, fmt.Errorf("read vLLM evidence manifest %s: %w", manifestEvidence.Platform, err)
		}
		digest := sha256.Sum256(body)
		if !strings.EqualFold(hex.EncodeToString(digest[:]), manifestEvidence.SHA256) {
			return nil, fmt.Errorf("vLLM evidence manifest %s SHA-256 mismatch", manifestEvidence.Platform)
		}
		manifest, err := vllm.ParseManifest(body)
		if err != nil {
			return nil, fmt.Errorf("vLLM evidence manifest %s: %w", manifestEvidence.Platform, err)
		}
		if err := validateProfileEvidence(manifest, manifestEvidence.SHA256, manifestEvidence.ProfileResults); err != nil {
			return nil, fmt.Errorf("vLLM evidence manifest %s: %w", manifestEvidence.Platform, err)
		}
		manifestPaths[manifestEvidence.Platform] = manifestPath
	}
	return manifestPaths, nil
}

func validateProfileEvidence(manifest vllm.Manifest, manifestSHA256 string, results []RuntimeProfileEvidence) error {
	expected := make(map[string]string)
	for _, profile := range manifest.Profiles {
		for _, operatingSystem := range profile.OperatingSystems {
			for _, architecture := range profile.Architectures {
				for _, device := range profile.Devices {
					key := evidenceResultKey(profile.ID, operatingSystem, architecture, device)
					expected[key] = profile.InstallMethod
				}
			}
		}
	}
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
		if strings.TrimSpace(result.Runner) == "" || len(result.Runner) > 256 {
			return fmt.Errorf("profile result %q has no hardware runner identity", key)
		}
		// A GitHub-hosted runner has no accelerator, so it can only attest a CPU
		// profile. Everything else still requires a protected self-hosted runner.
		switch result.RunnerClass {
		case "self-hosted":
		case "github-hosted":
			if result.Device != "cpu" {
				return fmt.Errorf("profile result %q was produced on a GitHub-hosted runner, which cannot validate a %s profile", key, result.Device)
			}
		default:
			return fmt.Errorf("profile result %q has unsupported runner class %q", key, result.RunnerClass)
		}
		if !strings.EqualFold(result.ManifestSHA256, manifestSHA256) {
			return fmt.Errorf("profile result %q does not match manifest SHA-256", key)
		}
		runURL, err := url.Parse(result.RunURL)
		runID := strings.TrimPrefix(result.RunURL, trustedProfileRunPrefix)
		if err != nil || runURL.RawQuery != "" || runURL.Fragment != "" || runURL.User != nil || runID == result.RunURL || !numericRunID(runID) {
			return fmt.Errorf("profile result %q has invalid run URL", key)
		}
		for check, status := range map[string]string{
			"installation": result.Installation,
			"import":       result.Import,
			"serve":        result.Serve,
			"Python audit": result.PythonDependencyAudit,
			"runtime scan": result.RuntimeScan,
		} {
			if status != "passed" {
				return fmt.Errorf("profile result %q %s did not pass", key, check)
			}
		}
		if installMethod == "oci" {
			if result.ContainerScan != "passed" {
				return fmt.Errorf("profile result %q container scan did not pass", key)
			}
		} else if result.ContainerScan != "passed" && result.ContainerScan != "not_applicable" {
			return fmt.Errorf("profile result %q has invalid container scan status", key)
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("profile evidence covers %d of %d platform/device combinations", len(seen), len(expected))
	}
	return nil
}

func numericRunID(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return value[0] != '0'
}

func evidenceResultKey(profileID string, operatingSystem string, architecture string, device string) string {
	return strings.Join([]string{strings.TrimSpace(profileID), strings.ToLower(strings.TrimSpace(operatingSystem)), strings.ToLower(strings.TrimSpace(architecture)), strings.ToLower(strings.TrimSpace(device))}, "/")
}

func readBoundedEvidenceFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, fmt.Errorf("%q is not a bounded regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Size() != info.Size() {
		return nil, fmt.Errorf("%q changed before reading", path)
	}
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	finished, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if int64(len(body)) != opened.Size() || finished.Size() != opened.Size() || !finished.ModTime().Equal(opened.ModTime()) {
		return nil, fmt.Errorf("%q changed while reading", path)
	}
	return body, nil
}

func requireEvidenceEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("vLLM runtime evidence contains trailing JSON")
		}
		return fmt.Errorf("decode vLLM runtime evidence trailing content: %w", err)
	}
	return nil
}

var commitPattern = regexp.MustCompile(`^(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$`)

func validCommit(value string) bool {
	return commitPattern.MatchString(strings.TrimSpace(value))
}
