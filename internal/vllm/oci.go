package vllm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ociMetadataFilename = "oci-runtime.json"

type ociRuntimeMetadata struct {
	Engine string `json:"engine"`
	Image  string `json:"image"`
}

type ociMount struct {
	Source      string
	Destination string
	ReadOnly    bool
}

func findContainerEngine(preferred string) (string, string, error) {
	candidates := []string{}
	if strings.TrimSpace(preferred) != "" {
		candidates = append(candidates, preferred)
	}
	candidates = append(candidates, "docker", "podman")
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		name := strings.ToLower(filepath.Base(strings.TrimSpace(candidate)))
		name = strings.TrimSuffix(name, filepath.Ext(name))
		if name != "docker" && name != "podman" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		path, err := findExecutable(candidate)
		if err == nil {
			return path, name, nil
		}
	}
	return "", "", fmt.Errorf("container engine prerequisite requires docker or podman")
}

func containerEngineEnvironment(environmentPath string) []string {
	return []string{
		"HOME=" + environmentPath,
		"DOCKER_CONFIG=" + filepath.Join(environmentPath, "docker-config"),
		"REGISTRY_AUTH_FILE=" + os.DevNull,
		"XDG_CONFIG_HOME=" + filepath.Join(environmentPath, "xdg-config"),
		"XDG_DATA_HOME=" + filepath.Join(environmentPath, "xdg-data"),
		"XDG_CACHE_HOME=" + filepath.Join(environmentPath, "xdg-cache"),
	}
}

func readOCIRuntimeMetadata(environmentPath string) (ociRuntimeMetadata, error) {
	var metadata ociRuntimeMetadata
	if err := readJSONRegular(filepath.Join(environmentPath, ociMetadataFilename), &metadata, 1<<20); err != nil {
		return ociRuntimeMetadata{}, fmt.Errorf("read OCI runtime metadata: %w", err)
	}
	if metadata.Engine != "docker" && metadata.Engine != "podman" || !validOCIImage(metadata.Image) {
		return ociRuntimeMetadata{}, fmt.Errorf("OCI runtime metadata is invalid")
	}
	return metadata, nil
}

func installedContainerEngine(profile Profile, environmentPath string) (string, error) {
	if profile.InstallMethod != "oci" {
		return "", nil
	}
	metadata, err := readOCIRuntimeMetadata(environmentPath)
	if err != nil {
		return "", err
	}
	if metadata.Image != profile.OCIImage {
		return "", fmt.Errorf("OCI runtime image does not match authorized profile")
	}
	return metadata.Engine, nil
}

func ociCommandArguments(engine string, profile Profile, mounts []ociMount, environment []string, command []string, network bool, runAsImageUser ...bool) []string {
	arguments := []string{"run", "--rm", "--pull=never", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--tmpfs", "/tmp:rw,nosuid,nodev,size=1g", "--shm-size=4g"}
	// By default the container runs as the host user so anything it writes stays
	// owned by that user and nothing runs as root. Some vendor images keep their
	// interpreter under /root and are unusable that way, so an operator can opt a
	// deployment back onto the image own user. Everything else stays: no new
	// privileges, all capabilities dropped, read-only root.
	if len(runAsImageUser) == 0 || !runAsImageUser[0] {
		arguments = append(arguments, containerIdentityArguments(engine)...)
	}
	if !network {
		arguments = append(arguments, "--network=none")
	}
	for _, device := range profile.Devices {
		switch strings.ToLower(device) {
		case "cuda":
			if engine == "podman" {
				arguments = append(arguments, "--device=nvidia.com/gpu=all")
			} else {
				arguments = append(arguments, "--gpus=all")
			}
		case "rocm":
			arguments = append(arguments, "--device=/dev/kfd", "--device=/dev/dri")
		case "xpu":
			arguments = append(arguments, "--device=/dev/dri")
		}
	}
	for _, mount := range mounts {
		value := "type=bind,src=" + mount.Source + ",dst=" + mount.Destination
		if mount.ReadOnly {
			value += ",readonly"
		}
		arguments = append(arguments, "--mount", value)
	}
	for _, value := range append(containerRuntimeEnvironment(), environment...) {
		arguments = append(arguments, "--env", value)
	}
	arguments = append(arguments, "--entrypoint=python3", profile.OCIImage)
	return append(arguments, command...)
}

func containerRuntimeEnvironment() []string {
	return []string{
		"HOME=/tmp/vllm",
		"HF_HOME=/tmp/vllm/huggingface",
		"HF_HUB_OFFLINE=1",
		"TRANSFORMERS_OFFLINE=1",
		"PIP_NO_INDEX=1",
		"VLLM_NO_USAGE_STATS=1",
		"DO_NOT_TRACK=1",
		"PYTHONNOUSERSITE=1",
		"PYTHONDONTWRITEBYTECODE=1",
	}
}
