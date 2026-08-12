package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"tensors-router/internal/vllm"
)

const profileCheckTimeout = 4 * time.Hour

type profileCheckConfig struct {
	DataDir              string
	ManifestPath         string
	Profile              string
	ExpectedOS           string
	ExpectedArchitecture string
	ExpectedDevice       string
}

type profileCheckResult struct {
	Profile        string `json:"profile"`
	RuntimeVersion string `json:"runtime_version"`
	DetectedDevice string `json:"detected_device"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

func runProfileCheck(arguments []string, output io.Writer) error {
	if !vllm.SupportedPlatform() {
		return fmt.Errorf("%s", vllm.UnsupportedReason())
	}
	configuration, err := parseProfileCheckConfig(arguments)
	if err != nil {
		return err
	}
	if runtime.GOOS != configuration.ExpectedOS || runtime.GOARCH != configuration.ExpectedArchitecture {
		return fmt.Errorf("profile runner is %s/%s, expected %s/%s", runtime.GOOS, runtime.GOARCH, configuration.ExpectedOS, configuration.ExpectedArchitecture)
	}
	detection, err := (vllm.SystemDetector{}).Detect(context.Background())
	if err != nil {
		return fmt.Errorf("detect profile runner hardware: %w", err)
	}
	if !containsFold(detection.Devices, configuration.ExpectedDevice) {
		return fmt.Errorf("profile runner did not detect required %s device", configuration.ExpectedDevice)
	}
	manifestSize, manifestDigest, err := profileManifestAuthorization(configuration.ManifestPath)
	if err != nil {
		return err
	}
	manager, err := vllm.NewManager(vllm.ManagerOptions{
		DataDir:        configuration.DataDir,
		DefaultProfile: configuration.Profile,
		ManifestSource: vllm.AuthorizedManifestFile{
			Path: configuration.ManifestPath,
			Authorization: vllm.ArtifactAuthorization{
				Length: manifestSize,
				SHA256: manifestDigest,
			},
		},
		Detector:    vllm.SystemDetector{},
		Downloader:  vllm.HTTPArtifactDownloader{},
		Installer:   vllm.UVEnvironmentInstaller{},
		SmokeTester: vllm.CommandSmokeTester{},
	})
	if err != nil {
		return err
	}
	defer manager.Close()
	checkContext, cancel := context.WithTimeout(context.Background(), profileCheckTimeout)
	defer cancel()
	if _, err := manager.StartInitialization(checkContext, vllm.InitRequest{Profile: configuration.Profile}); err != nil {
		return err
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		state := manager.State(checkContext)
		switch state.LifecycleState {
		case vllm.LifecycleReady:
			return json.NewEncoder(output).Encode(profileCheckResult{
				Profile:        state.SelectedProfile,
				RuntimeVersion: state.RuntimeVersion,
				DetectedDevice: state.DetectedProfile,
				ManifestSHA256: manifestDigest,
			})
		case vllm.LifecycleFailed:
			return fmt.Errorf("profile validation failed: %s", state.Error)
		}
		select {
		case <-checkContext.Done():
			_, _ = manager.CancelInitialization(context.Background())
			return checkContext.Err()
		case <-ticker.C:
		}
	}
}

func parseProfileCheckConfig(arguments []string) (profileCheckConfig, error) {
	var configuration profileCheckConfig
	for index := 0; index < len(arguments); index++ {
		name := arguments[index]
		if index+1 >= len(arguments) {
			return profileCheckConfig{}, fmt.Errorf("%s requires a value", name)
		}
		value := strings.TrimSpace(arguments[index+1])
		index++
		switch name {
		case "--data-dir":
			configuration.DataDir = value
		case "--manifest":
			configuration.ManifestPath = value
		case "--profile":
			configuration.Profile = value
		case "--expected-os":
			configuration.ExpectedOS = value
		case "--expected-architecture":
			configuration.ExpectedArchitecture = value
		case "--expected-device":
			configuration.ExpectedDevice = value
		default:
			return profileCheckConfig{}, fmt.Errorf("unknown profile-check option %q", name)
		}
	}
	if configuration.DataDir == "" || configuration.ManifestPath == "" || configuration.Profile == "" || configuration.ExpectedOS == "" || configuration.ExpectedArchitecture == "" || configuration.ExpectedDevice == "" {
		return profileCheckConfig{}, fmt.Errorf("profile-check requires data directory, manifest, profile, operating system, architecture, and device")
	}
	return configuration, nil
}

func profileManifestAuthorization(path string) (int64, string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > 4<<20 {
		return 0, "", fmt.Errorf("profile manifest must be a bounded regular file")
	}
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return 0, "", err
	}
	if int64(len(content)) != info.Size() {
		return 0, "", fmt.Errorf("profile manifest changed while reading")
	}
	digest := sha256.Sum256(content)
	return info.Size(), hex.EncodeToString(digest[:]), nil
}

func containsFold(values []string, expected string) bool {
	for _, value := range values {
		if strings.EqualFold(value, expected) {
			return true
		}
	}
	return false
}
