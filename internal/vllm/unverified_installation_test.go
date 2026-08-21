//go:build vllm_embedded_uv

package vllm

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnverifiedInstallationInvokesUVWithoutPinningOrOfflineFlags(t *testing.T) {
	directory := t.TempDir()
	environmentPath := filepath.Join(directory, "environment")
	if err := ensurePrivateDirectory(environmentPath); err != nil {
		t.Fatal(err)
	}
	profile := Profile{ID: UnverifiedProfileID, VLLMVersion: "0.6.3", PythonVersion: "3.12", InstallMethod: "pypi"}
	runner := &recordingCommandRunner{}
	installer := UVEnvironmentInstaller{Runner: runner, IndexURL: "https://download.pytorch.org/whl/cu124", ExtraIndexURL: "https://pypi.org/simple"}
	if err := installer.Install(context.Background(), profile, map[string]string{}, environmentPath, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 2 {
		t.Fatalf("expected exactly two commands (venv, pip install), got %d: %#v", len(runner.commands), runner.commands)
	}
	venvArguments := strings.Join(runner.commands[0].arguments, " ")
	if !strings.Contains(venvArguments, "venv --python 3.12") {
		t.Fatalf("expected uv venv with the pinned Python version, got: %s", venvArguments)
	}
	// The staged environment already contains the bootstrap directory, and uv exits 2
	// on a non-empty target without this flag.
	if !strings.Contains(venvArguments, "--allow-existing") {
		t.Fatalf("expected uv venv to tolerate the pre-created environment directory, got: %s", venvArguments)
	}
	installArguments := strings.Join(runner.commands[1].arguments, " ")
	if !strings.Contains(installArguments, "vllm==0.6.3") {
		t.Fatalf("expected the pinned vLLM version in the install command, got: %s", installArguments)
	}
	if !strings.Contains(installArguments, "--index-url https://download.pytorch.org/whl/cu124") || !strings.Contains(installArguments, "--extra-index-url https://pypi.org/simple") {
		t.Fatalf("expected configured index URLs to be passed through, got: %s", installArguments)
	}
	for _, forbidden := range []string{"--offline", "--no-index", "--no-deps", "--find-links"} {
		if strings.Contains(installArguments, forbidden) {
			t.Fatalf("unverified install must resolve dependencies online, but found %q in: %s", forbidden, installArguments)
		}
	}
	for _, command := range runner.commands {
		environment := strings.Join(command.environment, "\n")
		if strings.Contains(environment, "UV_OFFLINE=1") || strings.Contains(environment, "PIP_NO_INDEX=1") {
			t.Fatalf("unverified install environment must not be offline-restricted: %#v", command.environment)
		}
	}
}

func TestUnverifiedInstallationWithoutPinnedVersionInstallsLatest(t *testing.T) {
	directory := t.TempDir()
	environmentPath := filepath.Join(directory, "environment")
	if err := ensurePrivateDirectory(environmentPath); err != nil {
		t.Fatal(err)
	}
	profile := Profile{ID: UnverifiedProfileID, PythonVersion: "3.12", InstallMethod: "pypi"}
	runner := &recordingCommandRunner{}
	installer := UVEnvironmentInstaller{Runner: runner}
	if err := installer.Install(context.Background(), profile, map[string]string{}, environmentPath, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	installArguments := runner.commands[1].arguments
	if installArguments[len(installArguments)-1] != "vllm" {
		t.Fatalf("expected the final argument to be a bare \"vllm\" spec when no version is pinned, got: %v", installArguments)
	}
}
