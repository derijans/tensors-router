package vllm

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedVLLMImportCheckMatchesLocalVersionWheel(t *testing.T) {
	pythonPath, pythonVersion := requireRunnablePython(t)
	packagesDirectory := writeLocalVersionVLLMDistribution(t, "0.30.0", "0.30.0+cpu")

	matching := Profile{VLLMVersion: "0.30.0+cpu", PythonVersion: pythonVersion}
	if output, err := runPythonCheck(pythonPath, packagesDirectory, pinnedVLLMImportCheck(matching)); err != nil {
		t.Fatalf("pinned local-version wheel was rejected: %v: %s", err, output)
	}

	mismatched := Profile{VLLMVersion: "0.30.0+rocm723", PythonVersion: pythonVersion}
	if _, err := runPythonCheck(pythonPath, packagesDirectory, pinnedVLLMImportCheck(mismatched)); err == nil {
		t.Fatal("a different local-version pin was accepted")
	}
}

func requireRunnablePython(t *testing.T) (string, string) {
	t.Helper()
	for _, name := range []string{"python3", "python"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		output, err := exec.Command(path, "-I", "-c", "import sys; print('.'.join(map(str,sys.version_info[:3])))").Output()
		if err == nil {
			return path, strings.TrimSpace(string(output))
		}
	}
	t.Skip("no runnable Python interpreter on PATH")
	return "", ""
}

func writeLocalVersionVLLMDistribution(t *testing.T, moduleVersion string, distributionVersion string) string {
	t.Helper()
	directory := t.TempDir()
	moduleDirectory := filepath.Join(directory, "vllm")
	distributionDirectory := filepath.Join(directory, "vllm-"+distributionVersion+".dist-info")
	for _, path := range []string{moduleDirectory, distributionDirectory} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(moduleDirectory, "__init__.py"):    "__version__ = '" + moduleVersion + "'\n",
		filepath.Join(distributionDirectory, "METADATA"): "Metadata-Version: 2.1\nName: vllm\nVersion: " + distributionVersion + "\n",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func runPythonCheck(pythonPath string, packagesDirectory string, script string) ([]byte, error) {
	command := exec.Command(pythonPath, "-c", script)
	command.Dir = packagesDirectory
	return command.CombinedOutput()
}
