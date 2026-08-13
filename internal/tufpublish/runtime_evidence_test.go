package tufpublish

import (
	"strings"
	"testing"

	"tensors-router/internal/vllm"
)

func TestValidateProfileEvidenceRequiresEverySecurityAndSmokeGate(t *testing.T) {
	manifest := vllm.Manifest{Profiles: []vllm.Profile{{ID: "linux-amd64-cuda", InstallMethod: "wheel", OperatingSystems: []string{"linux"}, Architectures: []string{"amd64"}, Devices: []string{"cuda"}}}}
	result := passingRuntimeProfileEvidence()
	if err := validateProfileEvidence(manifest, strings.Repeat("a", 64), []RuntimeProfileEvidence{result}); err != nil {
		t.Fatal(err)
	}
	failures := []func(*RuntimeProfileEvidence){
		func(value *RuntimeProfileEvidence) { value.Installation = "failed" },
		func(value *RuntimeProfileEvidence) { value.Import = "failed" },
		func(value *RuntimeProfileEvidence) { value.Serve = "failed" },
		func(value *RuntimeProfileEvidence) { value.PythonDependencyAudit = "failed" },
		func(value *RuntimeProfileEvidence) { value.RuntimeScan = "failed" },
	}
	for _, mutate := range failures {
		candidate := result
		mutate(&candidate)
		if err := validateProfileEvidence(manifest, strings.Repeat("a", 64), []RuntimeProfileEvidence{candidate}); err == nil {
			t.Fatal("profile evidence accepted a failed required gate")
		}
	}
}

func TestValidateProfileEvidenceRequiresOCIContainerScan(t *testing.T) {
	manifest := vllm.Manifest{Profiles: []vllm.Profile{{ID: "linux-amd64-cuda", InstallMethod: "oci", OperatingSystems: []string{"linux"}, Architectures: []string{"amd64"}, Devices: []string{"cuda"}}}}
	result := passingRuntimeProfileEvidence()
	result.ContainerScan = "not_applicable"
	if err := validateProfileEvidence(manifest, strings.Repeat("a", 64), []RuntimeProfileEvidence{result}); err == nil {
		t.Fatal("OCI profile evidence without a passed container scan was accepted")
	}
}

func TestValidateProfileEvidenceRequiresEveryDeviceCombination(t *testing.T) {
	manifest := vllm.Manifest{Profiles: []vllm.Profile{{ID: "linux-amd64", InstallMethod: "wheel", OperatingSystems: []string{"linux"}, Architectures: []string{"amd64"}, Devices: []string{"cpu", "cuda"}}}}
	result := passingRuntimeProfileEvidence()
	result.ProfileID = "linux-amd64"
	if err := validateProfileEvidence(manifest, strings.Repeat("a", 64), []RuntimeProfileEvidence{result}); err == nil {
		t.Fatal("partial hardware evidence was accepted")
	}
}

func passingRuntimeProfileEvidence() RuntimeProfileEvidence {
	return RuntimeProfileEvidence{
		ProfileID:             "linux-amd64-cuda",
		OperatingSystem:       "linux",
		Architecture:          "amd64",
		Device:                "cuda",
		Runner:                "vendor-cuda-runner",
		RunURL:                "https://github.com/derijans/tensors-router/actions/runs/1",
		ManifestSHA256:        strings.Repeat("a", 64),
		Installation:          "passed",
		Import:                "passed",
		Serve:                 "passed",
		PythonDependencyAudit: "passed",
		RuntimeScan:           "passed",
		ContainerScan:         "not_applicable",
	}
}

func TestValidateProfileEvidenceRejectsUntrustedRunURL(t *testing.T) {
	manifest := vllm.Manifest{Profiles: []vllm.Profile{{ID: "linux-amd64-cuda", InstallMethod: "wheel", OperatingSystems: []string{"linux"}, Architectures: []string{"amd64"}, Devices: []string{"cuda"}}}}
	result := passingRuntimeProfileEvidence()
	result.RunURL = "https://example.com/passed"
	if err := validateProfileEvidence(manifest, strings.Repeat("a", 64), []RuntimeProfileEvidence{result}); err == nil {
		t.Fatal("untrusted profile run URL was accepted")
	}
}

func TestValidateProfileEvidenceBindsReceiptToManifest(t *testing.T) {
	manifest := vllm.Manifest{Profiles: []vllm.Profile{{ID: "linux-amd64-cuda", InstallMethod: "wheel", OperatingSystems: []string{"linux"}, Architectures: []string{"amd64"}, Devices: []string{"cuda"}}}}
	result := passingRuntimeProfileEvidence()
	if err := validateProfileEvidence(manifest, strings.Repeat("b", 64), []RuntimeProfileEvidence{result}); err == nil {
		t.Fatal("profile receipt for different manifest was accepted")
	}
}
