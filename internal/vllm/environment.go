package vllm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
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

type SystemDetector struct {
	operatingSystem     string
	architecture        string
	deviceExists        func(string) bool
	intelGPUDetected    func() bool
	prerequisiteCommand func(context.Context, string, ...string) bool
}

func (detector SystemDetector) Detect(ctx context.Context) (Detection, error) {
	operatingSystem := detector.operatingSystem
	if operatingSystem == "" {
		operatingSystem = runtime.GOOS
	}
	architecture := detector.architecture
	if architecture == "" {
		architecture = runtime.GOARCH
	}
	deviceExists := detector.deviceExists
	if deviceExists == nil {
		deviceExists = regularOrDeviceExists
	}
	intelDetected := detector.intelGPUDetected
	if intelDetected == nil {
		intelDetected = intelGPUDetected
	}
	commandSucceeds := detector.prerequisiteCommand
	if commandSucceeds == nil {
		commandSucceeds = prerequisiteCommandSucceeds
	}

	detection := Detection{OS: operatingSystem, Architecture: architecture, Devices: []string{"cpu"}, Prerequisites: map[string]bool{}}
	if operatingSystem == "darwin" && architecture == "arm64" {
		detection.Devices = append([]string{"metal"}, detection.Devices...)
		detection.Prerequisites["metal"] = true
	}
	if deviceExists("/dev/nvidiactl") {
		detection.Devices = append([]string{"cuda"}, detection.Devices...)
		detection.Prerequisites["nvidia_driver"] = commandSucceeds(ctx, "nvidia-smi", "-L")
	}
	if deviceExists("/dev/kfd") {
		detection.Devices = append([]string{"rocm"}, detection.Devices...)
		detection.Prerequisites["rocm_driver"] = commandSucceeds(ctx, "rocminfo") || commandSucceeds(ctx, "amd-smi", "list") || commandSucceeds(ctx, "rocm-smi", "--showid")
	}
	if intelDetected() {
		detection.Devices = append([]string{"xpu"}, detection.Devices...)
		detection.Prerequisites["intel_gpu"] = commandSucceeds(ctx, "xpu-smi", "discovery") || commandSucceeds(ctx, "sycl-ls")
	}
	detection.Prerequisites["container_engine"] = executableExists("docker") || executableExists("podman")
	detection.Prerequisites["compiler"] = executableExists("cc") || executableExists("clang") || executableExists("gcc")
	return detection, nil
}

func prerequisiteCommandSucceeds(ctx context.Context, name string, arguments ...string) bool {
	path, err := findExecutable(name)
	if err != nil {
		return false
	}
	commandContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(commandContext, path, arguments...)
	command.Stdin = nil
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run() == nil
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
