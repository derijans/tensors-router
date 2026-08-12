package vllm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func profileArtifactBytes(profile Profile) (int64, error) {
	var total int64
	for _, artifact := range profile.Artifacts {
		if artifact.Size > int64(^uint64(0)>>1)-total {
			return 0, fmt.Errorf("vLLM profile artifact sizes overflow")
		}
		total += artifact.Size
	}
	return total, nil
}

func environmentID(profile Profile, manifestDigest string) (string, error) {
	content, err := json.Marshal(profile)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(manifestDigest))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write(content)
	return profile.ID + "-" + hex.EncodeToString(digest.Sum(nil))[:24], nil
}

func sanitizePhase(phase string) string {
	phase = strings.TrimSpace(phase)
	if !safeIdentifier(phase) {
		return "installing"
	}
	return phase
}

func sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(redactSensitive(strings.TrimSpace(err.Error())))
}

type SystemDetector struct{}

func (SystemDetector) Detect(context.Context) (Detection, error) {
	detection := Detection{OS: runtime.GOOS, Architecture: runtime.GOARCH, Devices: []string{"cpu"}, Prerequisites: map[string]bool{}}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		detection.Devices = append([]string{"metal"}, detection.Devices...)
		detection.Prerequisites["metal"] = true
	}
	if regularOrDeviceExists("/dev/nvidiactl") {
		detection.Devices = append([]string{"cuda"}, detection.Devices...)
		detection.Prerequisites["nvidia_driver"] = executableExists("nvidia-smi")
	}
	if regularOrDeviceExists("/dev/kfd") {
		detection.Devices = append([]string{"rocm"}, detection.Devices...)
		detection.Prerequisites["rocm_driver"] = true
	}
	if intelGPUDetected() {
		detection.Devices = append([]string{"xpu"}, detection.Devices...)
		detection.Prerequisites["intel_gpu"] = true
	}
	detection.Prerequisites["container_engine"] = executableExists("docker") || executableExists("podman")
	detection.Prerequisites["compiler"] = executableExists("cc") || executableExists("clang") || executableExists("gcc")
	return detection, nil
}

func intelGPUDetected() bool {
	vendors, err := filepath.Glob("/sys/class/drm/card*/device/vendor")
	if err != nil {
		return false
	}
	for _, vendorPath := range vendors {
		content, err := os.ReadFile(vendorPath)
		if err == nil && strings.EqualFold(strings.TrimSpace(string(content)), "0x8086") {
			return true
		}
	}
	return false
}

func regularOrDeviceExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func executableExists(name string) bool {
	_, err := findExecutable(name)
	return err == nil
}
