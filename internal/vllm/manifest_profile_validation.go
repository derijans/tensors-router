package vllm

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

func validateProfile(profile Profile) error {
	for _, validate := range []func(Profile) error{
		validateProfileVersions,
		validateProfilePlatforms,
		validateProfilePrerequisites,
		validateProfileArtifacts,
	} {
		if err := validate(profile); err != nil {
			return err
		}
	}
	return nil
}

func validateProfileVersions(profile Profile) error {
	if !safeIdentifier(profile.ID) {
		return fmt.Errorf("invalid profile id %q", profile.ID)
	}
	if err := validatePinnedVersion("vLLM version", profile.VLLMVersion); err != nil {
		return err
	}
	if err := validatePinnedVersion("Python version", profile.PythonVersion); err != nil {
		return err
	}
	for plugin, version := range profile.PluginVersions {
		if !safePackageName(plugin) {
			return fmt.Errorf("invalid plugin name %q", plugin)
		}
		if err := validatePinnedVersion("plugin "+plugin+" version", version); err != nil {
			return err
		}
	}
	return nil
}

func validateProfilePlatforms(profile Profile) error {
	switch profile.InstallMethod {
	case "wheel", "source", "oci":
	default:
		return fmt.Errorf("unsupported installation method %q", profile.InstallMethod)
	}
	if len(profile.OperatingSystems) == 0 || len(profile.Architectures) == 0 || len(profile.Devices) == 0 {
		return fmt.Errorf("operating_systems, architectures, and devices are required")
	}
	if err := validateSet(profile.OperatingSystems, validOS); err != nil {
		return fmt.Errorf("operating systems: %w", err)
	}
	if err := validateSet(profile.Architectures, validArchitecture); err != nil {
		return fmt.Errorf("architectures: %w", err)
	}
	if err := validateSet(profile.Devices, validDevice); err != nil {
		return fmt.Errorf("devices: %w", err)
	}
	return nil
}

func validateProfilePrerequisites(profile Profile) error {
	prerequisites := make(map[string]struct{}, len(profile.Prerequisites))
	for _, prerequisite := range profile.Prerequisites {
		if !safeIdentifier(prerequisite.ID) || strings.TrimSpace(prerequisite.Description) == "" {
			return fmt.Errorf("invalid prerequisite %q", prerequisite.ID)
		}
		if !supportedPrerequisite(prerequisite.ID) {
			return fmt.Errorf("unsupported prerequisite %q", prerequisite.ID)
		}
		if _, exists := prerequisites[prerequisite.ID]; exists {
			return fmt.Errorf("duplicate prerequisite %q", prerequisite.ID)
		}
		prerequisites[prerequisite.ID] = struct{}{}
	}
	return nil
}

func validateProfileArtifacts(profile Profile) error {
	if len(profile.Artifacts) == 0 {
		return fmt.Errorf("artifacts are required")
	}
	artifactNames := make(map[string]struct{}, len(profile.Artifacts))
	artifactRoles := make(map[string]int)
	for _, artifact := range profile.Artifacts {
		if err := validateArtifact(artifact); err != nil {
			return err
		}
		if _, exists := artifactNames[artifact.Name]; exists {
			return fmt.Errorf("duplicate artifact %q", artifact.Name)
		}
		artifactNames[artifact.Name] = struct{}{}
		artifactRoles[artifact.Role]++
	}
	if err := validateArtifactRoles(profile.InstallMethod, artifactRoles); err != nil {
		return err
	}
	if artifactRoles["smoke_model"] != 1 {
		return fmt.Errorf("%s profile requires exactly one signed smoke-model artifact", profile.InstallMethod)
	}
	if profile.InstallMethod == "oci" {
		return validateOCIProfileArtifacts(profile, artifactRoles)
	}
	return validatePackageProfileArtifacts(profile, artifactRoles)
}

func validateOCIProfileArtifacts(profile Profile, artifactRoles map[string]int) error {
	if artifactRoles["oci"] != 1 {
		return fmt.Errorf("OCI profile requires exactly one OCI artifact")
	}
	if !validOCIImage(profile.OCIImage) {
		return fmt.Errorf("OCI profile requires an immutable sha256 image id")
	}
	if !profileHasPrerequisite(profile, "container_engine") {
		return fmt.Errorf("OCI profile requires container_engine prerequisite")
	}
	return nil
}

func validatePackageProfileArtifacts(profile Profile, artifactRoles map[string]int) error {
	if profile.OCIImage != "" {
		return fmt.Errorf("oci_image is only valid for OCI profiles")
	}
	if artifactRoles["uv"] > 1 || artifactRoles["python"] != 1 {
		return fmt.Errorf("%s profile requires exactly one Python artifact and at most one uv fallback", profile.InstallMethod)
	}
	if artifactRoles["plugin"] != len(profile.PluginVersions) {
		return fmt.Errorf("%s profile must provide one plugin artifact for every pinned plugin version", profile.InstallMethod)
	}
	if profile.InstallMethod == "wheel" && artifactRoles["vllm"] != 1 {
		return fmt.Errorf("wheel profile requires exactly one vLLM artifact")
	}
	if profile.InstallMethod == "source" && artifactRoles["source"] != 1 {
		return fmt.Errorf("source profile requires exactly one source artifact")
	}
	if profile.InstallMethod == "source" && !profileHasPrerequisite(profile, "compiler") {
		return fmt.Errorf("source profile requires compiler prerequisite")
	}
	return nil
}

func validateArtifact(artifact Artifact) error {
	if err := validateArtifactSource(artifact); err != nil {
		return err
	}
	switch artifact.Role {
	case "python", "uv", "vllm", "plugin", "source", "oci", "dependency", "smoke_model":
	default:
		return fmt.Errorf("artifact %q has unsupported role %q", artifact.Name, artifact.Role)
	}
	if err := validateArtifactArchive(artifact); err != nil {
		return err
	}
	return validateArtifactExecutable(artifact)
}

func validateArtifactSource(artifact Artifact) error {
	if artifact.Name == "" || artifact.Name != filepath.Base(artifact.Name) || strings.ContainsAny(artifact.Name, `/\`) {
		return fmt.Errorf("invalid artifact name %q", artifact.Name)
	}
	parsed, err := url.Parse(artifact.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("artifact %q must use an absolute credential-free HTTPS URL", artifact.Name)
	}
	if artifact.Size <= 0 {
		return fmt.Errorf("artifact %q has invalid size %d", artifact.Name, artifact.Size)
	}
	if !validSHA256(artifact.SHA256) {
		return fmt.Errorf("artifact %q has invalid SHA-256", artifact.Name)
	}
	return nil
}

func validateArtifactArchive(artifact Artifact) error {
	if artifact.Role != "smoke_model" && artifact.Role != "python" {
		if artifact.ArchiveFormat != "" || artifact.UnpackedSize != 0 || artifact.ExecutablePath != "" {
			return fmt.Errorf("artifact %q cannot contain archive metadata", artifact.Name)
		}
		return nil
	}
	if artifact.ArchiveFormat != "tar" && artifact.ArchiveFormat != "tar.gz" {
		return fmt.Errorf("artifact %q requires archive_format tar or tar.gz", artifact.Name)
	}
	if artifact.UnpackedSize <= 0 {
		return fmt.Errorf("artifact %q requires positive unpacked_size", artifact.Name)
	}
	return nil
}

func validateArtifactExecutable(artifact Artifact) error {
	if artifact.Role != "python" {
		if artifact.ExecutablePath != "" {
			return fmt.Errorf("artifact %q executable_path is only valid for Python", artifact.Name)
		}
		return nil
	}
	if _, err := normalizePortableArchivePath(artifact.ExecutablePath); err != nil {
		return fmt.Errorf("Python artifact %q executable_path: %w", artifact.Name, err)
	}
	return nil
}
