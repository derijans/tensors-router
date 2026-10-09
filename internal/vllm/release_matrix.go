package vllm

import (
	"fmt"
	"sort"
	"strings"
)

var requiredReleaseProfiles = map[string][]string{
	"linux-amd64":  {"cuda", "rocm", "xpu", "cpu"},
	"linux-arm64":  {"cuda", "cpu"},
	"darwin-arm64": {"metal", "cpu"},
}

type releaseMatrixCheck struct {
	release string
	version string
}

func ValidateReleaseProfileMatrix(manifests map[string]Manifest) error {
	platforms := make([]string, 0, len(requiredReleaseProfiles))
	for platform := range requiredReleaseProfiles {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	var check releaseMatrixCheck
	for _, platform := range platforms {
		manifest, found := manifests[platform]
		if !found {
			return fmt.Errorf("vLLM release matrix is missing %s", platform)
		}
		if err := check.platform(platform, manifest); err != nil {
			return err
		}
	}
	return nil
}

func (check *releaseMatrixCheck) platform(platform string, manifest Manifest) error {
	if err := ValidateManifest(manifest); err != nil {
		return fmt.Errorf("vLLM release matrix %s: %w", platform, err)
	}
	if check.release == "" {
		check.release = manifest.Release
	} else if manifest.Release != check.release {
		return fmt.Errorf("vLLM release matrix mixes release %q with %q", check.release, manifest.Release)
	}
	operatingSystem, architecture, _ := strings.Cut(platform, "-")
	for _, device := range requiredReleaseProfiles[platform] {
		profile, found := releaseProfileForDevice(manifest, operatingSystem, architecture, device)
		if !found {
			return fmt.Errorf("vLLM release matrix %s is missing %s profile", platform, device)
		}
		if err := check.deviceProfile(platform, device, profile); err != nil {
			return err
		}
	}
	return nil
}

func (check *releaseMatrixCheck) deviceProfile(platform string, device string, profile Profile) error {
	if check.version == "" {
		check.version = profile.VLLMVersion
	} else if profile.VLLMVersion != check.version {
		return fmt.Errorf("vLLM release matrix mixes vLLM version %q with %q", check.version, profile.VLLMVersion)
	}
	if device != "metal" {
		return nil
	}
	if _, found := profile.PluginVersions["vllm-metal"]; !found {
		return fmt.Errorf("vLLM release matrix %s Metal profile must pin vllm-metal", platform)
	}
	return nil
}

func releaseProfileForDevice(manifest Manifest, operatingSystem string, architecture string, device string) (Profile, bool) {
	for _, profile := range manifest.Profiles {
		if containsFold(profile.OperatingSystems, operatingSystem) && containsFold(profile.Architectures, architecture) && containsFold(profile.Devices, device) {
			return profile, true
		}
	}
	return Profile{}, false
}
