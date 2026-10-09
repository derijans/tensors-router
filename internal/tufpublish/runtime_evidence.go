package tufpublish

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
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
