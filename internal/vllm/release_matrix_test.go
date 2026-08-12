package vllm

import "testing"

func TestValidateReleaseProfileMatrixRequiresCompleteUpstreamPlatforms(t *testing.T) {
	base := testManifest()
	profile := base.Profiles[0]
	profile.Prerequisites = nil
	profile.Devices = []string{"cpu"}
	linuxAMD64 := base
	linuxAMD64.Profiles = []Profile{
		releaseProfile(profile, "linux-amd64-cpu", "linux", "amd64", "cpu"),
		releaseProfile(profile, "linux-amd64-cuda", "linux", "amd64", "cuda"),
		releaseProfile(profile, "linux-amd64-rocm", "linux", "amd64", "rocm"),
		releaseProfile(profile, "linux-amd64-xpu", "linux", "amd64", "xpu"),
	}
	linuxARM64 := base
	linuxARM64.Profiles = []Profile{
		releaseProfile(profile, "linux-arm64-cpu", "linux", "arm64", "cpu"),
		releaseProfile(profile, "linux-arm64-cuda", "linux", "arm64", "cuda"),
	}
	darwinARM64 := base
	metal := releaseProfile(profile, "darwin-arm64-metal", "darwin", "arm64", "metal")
	metal.PluginVersions = map[string]string{"vllm-metal": "0.1.0"}
	metal.Artifacts = append(append([]Artifact{}, metal.Artifacts...), Artifact{Name: "vllm-metal.whl", URL: "https://example.test/vllm-metal.whl", Size: 2, SHA256: metal.Artifacts[0].SHA256, Role: "plugin"})
	darwinARM64.Profiles = []Profile{
		releaseProfile(profile, "darwin-arm64-cpu", "darwin", "arm64", "cpu"),
		metal,
	}
	manifests := map[string]Manifest{"linux-amd64": linuxAMD64, "linux-arm64": linuxARM64, "darwin-arm64": darwinARM64}
	if err := ValidateReleaseProfileMatrix(manifests); err != nil {
		t.Fatal(err)
	}
	delete(manifests, "linux-arm64")
	if err := ValidateReleaseProfileMatrix(manifests); err == nil {
		t.Fatal("incomplete release matrix was accepted")
	}
}

func releaseProfile(base Profile, id string, operatingSystem string, architecture string, device string) Profile {
	base.ID = id
	base.OperatingSystems = []string{operatingSystem}
	base.Architectures = []string{architecture}
	base.Devices = []string{device}
	return base
}
