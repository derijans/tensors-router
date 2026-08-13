package vllm

import (
	"fmt"
	"sort"
)

var requiredReleaseProfiles = map[string][]string{
	"linux-amd64":  {"cuda", "rocm", "xpu", "cpu"},
	"linux-arm64":  {"cuda", "cpu"},
	"darwin-arm64": {"metal", "cpu"},
}

func ValidateReleaseProfileMatrix(manifests map[string]Manifest) error {
	platforms := make([]string, 0, len(requiredReleaseProfiles))
	for platform := range requiredReleaseProfiles {
		platforms = append(platforms, platform)
	}
	sort.Strings(platforms)
	release := ""
	version := ""
	for _, platform := range platforms {
		manifest, found := manifests[platform]
		if !found {
			return fmt.Errorf("vLLM release matrix is missing %s", platform)
		}
		if err := ValidateManifest(manifest); err != nil {
			return fmt.Errorf("vLLM release matrix %s: %w", platform, err)
		}
		if release == "" {
			release = manifest.Release
		} else if manifest.Release != release {
			return fmt.Errorf("vLLM release matrix mixes release %q with %q", release, manifest.Release)
		}
		operatingSystem, architecture := platform[:len(platform)-len("-amd64")], "amd64"
		if platform == "linux-arm64" || platform == "darwin-arm64" {
			operatingSystem, architecture = platform[:len(platform)-len("-arm64")], "arm64"
		}
		for _, device := range requiredReleaseProfiles[platform] {
			profile, found := releaseProfileForDevice(manifest, operatingSystem, architecture, device)
			if !found {
				return fmt.Errorf("vLLM release matrix %s is missing %s profile", platform, device)
			}
			if version == "" {
				version = profile.VLLMVersion
			} else if profile.VLLMVersion != version {
				return fmt.Errorf("vLLM release matrix mixes vLLM version %q with %q", version, profile.VLLMVersion)
			}
			if device == "metal" {
				if _, found := profile.PluginVersions["vllm-metal"]; !found {
					return fmt.Errorf("vLLM release matrix %s Metal profile must pin vllm-metal", platform)
				}
			}
		}
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
