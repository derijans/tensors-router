package vllm

import (
	"fmt"
	"path/filepath"
	"strings"
)

func validatePersistentJob(job InitializationJob) error {
	if job.JobID == "" {
		if job.BackendID == "" && job.State == "" {
			return nil
		}
		return fmt.Errorf("job id is required")
	}
	if !safeIdentifier(job.JobID) || job.BackendID != BackendID {
		return fmt.Errorf("job identity is invalid")
	}
	switch job.State {
	case JobQueued, JobRunning, JobCompleted, JobFailed, JobCancelled:
	default:
		return fmt.Errorf("job state %q is invalid", job.State)
	}
	if err := validatePersistentJobLabels(job); err != nil {
		return err
	}
	return validatePersistentJobProgress(job)
}

func validatePersistentJobLabels(job InitializationJob) error {
	if job.SelectedProfile != "auto" && !safeIdentifier(job.SelectedProfile) {
		return fmt.Errorf("selected profile is invalid")
	}
	if job.ManifestSHA256 != "" && !validSHA256(job.ManifestSHA256) {
		return fmt.Errorf("manifest digest is invalid")
	}
	if job.Phase != "" && !safeIdentifier(job.Phase) {
		return fmt.Errorf("job phase is invalid")
	}
	if strings.ContainsAny(job.DetectedProfile, "\x00\r\n") {
		return fmt.Errorf("detected profile is invalid")
	}
	return nil
}

func validatePersistentJobProgress(job InitializationJob) error {
	if job.CompletedBytes < 0 || job.TotalBytes < 0 || job.TotalBytes > 0 && job.CompletedBytes > job.TotalBytes {
		return fmt.Errorf("job progress is invalid")
	}
	if job.CreatedAt.IsZero() || job.UpdatedAt.IsZero() || job.UpdatedAt.Before(job.CreatedAt) {
		return fmt.Errorf("job timestamps are invalid")
	}
	return nil
}

func validateActiveEnvironment(dataDir string, active activeEnvironment) error {
	if err := validateActiveEnvironmentIdentity(active); err != nil {
		return err
	}
	if err := validateActiveInstallMethod(active); err != nil {
		return err
	}
	for _, device := range active.Devices {
		if !validDevice(strings.ToLower(strings.TrimSpace(device))) {
			return fmt.Errorf("active vLLM device is invalid")
		}
	}
	return validateActiveEnvironmentMarker(dataDir, active)
}

func validateActiveEnvironmentIdentity(active activeEnvironment) error {
	if !safeIdentifier(active.ProfileID) || !validSHA256(active.ManifestSHA256) {
		return fmt.Errorf("active vLLM environment identity is invalid")
	}
	// An unverified PyPI install with no pinned version has no exact version to
	// validate here; every other install method still requires one.
	unpinnedPyPIInstall := active.InstallMethod == "pypi" && active.VLLMVersion == ""
	if !unpinnedPyPIInstall && validatePinnedVersion("vLLM version", active.VLLMVersion) != nil {
		return fmt.Errorf("active vLLM environment identity is invalid")
	}
	return nil
}

func validateActiveInstallMethod(active activeEnvironment) error {
	switch active.InstallMethod {
	case "", "wheel", "source", "pypi":
		if active.OCIImage != "" || active.ContainerEngine != "" {
			return fmt.Errorf("non-OCI environment contains OCI metadata")
		}
		return nil
	case "oci":
		if !validOCIImage(active.OCIImage) || active.ContainerEngine != "docker" && active.ContainerEngine != "podman" {
			return fmt.Errorf("active OCI environment metadata is invalid")
		}
		return nil
	default:
		return fmt.Errorf("active vLLM installation method is invalid")
	}
}

func validateActiveEnvironmentMarker(dataDir string, active activeEnvironment) error {
	if err := requirePathWithin(filepath.Join(dataDir, "environments"), active.Path); err != nil {
		return err
	}
	var marker environmentMarker
	if err := readJSONRegular(filepath.Join(active.Path, "environment.json"), &marker, 1<<20); err != nil {
		return err
	}
	if !environmentMarkerMatches(marker, active) {
		return fmt.Errorf("active vLLM environment marker does not match state")
	}
	return nil
}

func environmentMarkerMatches(marker environmentMarker, active activeEnvironment) bool {
	return marker.ProfileID == active.ProfileID &&
		marker.VLLMVersion == active.VLLMVersion &&
		equalSHA256(marker.ManifestSHA256, active.ManifestSHA256) &&
		marker.InstallMethod == active.InstallMethod &&
		marker.OCIImage == active.OCIImage &&
		marker.ContainerEngine == active.ContainerEngine &&
		equalStrings(marker.Devices, active.Devices)
}
