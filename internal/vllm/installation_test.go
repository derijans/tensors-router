package vllm

import (
	"archive/tar"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type recordedCommand struct {
	name        string
	arguments   []string
	environment []string
}

type recordingCommandRunner struct {
	commands []recordedCommand
}

type smokeRuntimeLauncher struct {
	commands []recordedCommand
	status   int
}

func (launcher *smokeRuntimeLauncher) Start(_ context.Context, name string, arguments []string, environment []string, directory string, _ io.Writer) (RuntimeChild, error) {
	launcher.commands = append(launcher.commands, recordedCommand{name: name, arguments: append([]string{}, arguments...), environment: append([]string{}, environment...)})
	socketPath := smokeSocketPathFromArguments(arguments)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/health" {
			http.NotFound(writer, request)
			return
		}
		status := launcher.status
		if status == 0 {
			status = http.StatusOK
		}
		writer.WriteHeader(status)
	})}
	child := &smokeRuntimeChild{server: server, done: make(chan struct{})}
	go func() {
		child.mu.Lock()
		child.err = server.Serve(listener)
		child.mu.Unlock()
		close(child.done)
	}()
	return child, nil
}

func smokeSocketPathFromArguments(arguments []string) string {
	for index, argument := range arguments {
		if argument == "--uds" && index+1 < len(arguments) {
			return arguments[index+1]
		}
		if strings.HasPrefix(argument, "type=bind,src=") && strings.HasSuffix(argument, ",dst=/router-smoke") {
			return filepath.Join(strings.TrimSuffix(strings.TrimPrefix(argument, "type=bind,src="), ",dst=/router-smoke"), "vllm.sock")
		}
	}
	return ""
}

type smokeRuntimeChild struct {
	server *http.Server
	done   chan struct{}
	mu     sync.Mutex
	err    error
}

func (child *smokeRuntimeChild) Wait() error {
	<-child.done
	child.mu.Lock()
	defer child.mu.Unlock()
	if child.err == http.ErrServerClosed {
		return nil
	}
	return child.err
}

func (child *smokeRuntimeChild) Stop(ctx context.Context) error {
	return child.server.Shutdown(ctx)
}

func (runner *recordingCommandRunner) Run(_ context.Context, name string, arguments []string, environment []string, _ string, _ io.Writer) error {
	runner.commands = append(runner.commands, recordedCommand{name: name, arguments: append([]string{}, arguments...), environment: append([]string{}, environment...)})
	return nil
}

func TestOCIInstallationImportsVerifiesAndSmokeTestsAuthorizedImage(t *testing.T) {
	directory := t.TempDir()
	enginePath := filepath.Join(directory, "docker")
	if runtimeExecutableSuffix() != "" {
		enginePath += runtimeExecutableSuffix()
	}
	if err := os.WriteFile(enginePath, []byte("engine"), 0o700); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(directory, "source.oci")
	if err := os.WriteFile(artifactPath, []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	smokeArchivePath, smokeSize := writeSmokeModelArchive(t, directory)
	environmentPath := filepath.Join(directory, "environment")
	if err := ensurePrivateDirectory(environmentPath); err != nil {
		t.Fatal(err)
	}
	profile := Profile{
		ID: "oci", VLLMVersion: "0.10.0", PythonVersion: "3.12.8", InstallMethod: "oci", OCIImage: "sha256:" + strings.Repeat("a", 64), Devices: []string{"cpu"},
		Artifacts: []Artifact{{Name: "runtime.oci", Role: "oci"}, {Name: "smoke-model.tar", Role: "smoke_model", ArchiveFormat: "tar", UnpackedSize: smokeSize}},
	}
	runner := &recordingCommandRunner{}
	installer := UVEnvironmentInstaller{Runner: runner, ContainerEngine: enginePath}
	if err := installer.Install(context.Background(), profile, map[string]string{"runtime.oci": artifactPath, "smoke-model.tar": smokeArchivePath}, environmentPath, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 2 || strings.Join(runner.commands[0].arguments, " ") != "load --input "+filepath.Join(environmentPath, "runtime.oci") || strings.Join(runner.commands[1].arguments, " ") != "image inspect "+profile.OCIImage {
		t.Fatalf("unexpected OCI installation commands %#v", runner.commands)
	}
	metadata, err := readOCIRuntimeMetadata(environmentPath)
	if err != nil || metadata.Engine != "docker" || metadata.Image != profile.OCIImage {
		t.Fatalf("unexpected OCI metadata %#v error=%v", metadata, err)
	}
	runner.commands = nil
	launcher := &smokeRuntimeLauncher{}
	tester := CommandSmokeTester{Runner: runner, Launcher: launcher, ContainerEngine: enginePath}
	if err := tester.Test(context.Background(), profile, environmentPath); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 1 || len(launcher.commands) != 1 {
		t.Fatalf("unexpected OCI smoke command count %d", len(runner.commands))
	}
	for _, command := range launcher.commands {
		joined := strings.Join(command.arguments, " ")
		for _, expected := range []string{"run --rm", "--pull=never", "--network=none", "--read-only", "--entrypoint=python3", profile.OCIImage} {
			if !strings.Contains(joined, expected) {
				t.Fatalf("OCI smoke command missing %q: %s", expected, joined)
			}
		}
		if runtimeIdentityExpected() && !strings.Contains(joined, "--user ") {
			t.Fatalf("OCI smoke command does not preserve host socket ownership: %s", joined)
		}
	}
}

func TestWheelInstallationKeepsCompilerPathAndUsesOnlyLocalArtifacts(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("PATH", filepath.Join(directory, "toolchain"))
	artifactsDirectory := filepath.Join(directory, "artifacts")
	if err := os.Mkdir(artifactsDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, name := range []string{"uv", "vllm.whl", "dependency.whl"} {
		path := filepath.Join(artifactsDirectory, name)
		if err := os.WriteFile(path, []byte(name), 0o700); err != nil {
			t.Fatal(err)
		}
		paths[name] = path
	}
	pythonArchivePath, pythonSize, pythonExecutablePath := writePythonRuntimeArchive(t, artifactsDirectory)
	paths["python.tar"] = pythonArchivePath
	smokeArchivePath, smokeSize := writeSmokeModelArchive(t, directory)
	paths["smoke-model.tar"] = smokeArchivePath
	profile := Profile{
		ID: "wheel", VLLMVersion: "0.10.0", PythonVersion: "3.12.8", InstallMethod: "wheel",
		Artifacts: []Artifact{{Name: "uv", Role: "uv"}, {Name: "python.tar", Role: "python", ArchiveFormat: "tar", UnpackedSize: pythonSize, ExecutablePath: pythonExecutablePath}, {Name: "vllm.whl", Role: "vllm"}, {Name: "dependency.whl", Role: "dependency"}, {Name: "smoke-model.tar", Role: "smoke_model", ArchiveFormat: "tar", UnpackedSize: smokeSize}},
	}
	runner := &recordingCommandRunner{}
	environmentPath := filepath.Join(directory, "environment")
	if err := ensurePrivateDirectory(environmentPath); err != nil {
		t.Fatal(err)
	}
	installer := UVEnvironmentInstaller{Runner: runner}
	if err := installer.Install(context.Background(), profile, paths, environmentPath, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(runner.commands) != 3 {
		t.Fatalf("unexpected command count %d", len(runner.commands))
	}
	if versionArguments := strings.Join(runner.commands[0].arguments, " "); !strings.Contains(versionArguments, "sys.version_info") || !strings.Contains(versionArguments, profile.PythonVersion) {
		t.Fatalf("Python runtime version was not validated exactly: %s", versionArguments)
	}
	installArguments := strings.Join(runner.commands[2].arguments, " ")
	if !strings.Contains(installArguments, "--offline --no-index --no-deps --find-links "+artifactsDirectory) {
		t.Fatalf("installer did not constrain package resolution to signed artifacts: %s", installArguments)
	}
	if !strings.Contains(strings.Join(runner.commands[0].environment, "\n"), "PATH="+filepath.Join(directory, "toolchain")) {
		t.Fatalf("source-build toolchain path was not preserved: %#v", runner.commands[0].environment)
	}
}

func writeSmokeModelArchive(t *testing.T, directory string) (string, int64) {
	t.Helper()
	path := filepath.Join(directory, "smoke-model.tar")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	content := []byte(`{"model_type":"gpt2"}`)
	if err := writer.WriteHeader(&tar.Header{Name: "config.json", Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path, int64(len(content))
}

func writePythonRuntimeArchive(t *testing.T, directory string) (string, int64, string) {
	t.Helper()
	executablePath := "bin/python"
	if runtimeExecutableSuffix() != "" {
		executablePath = "python.exe"
	}
	content := []byte("portable-python")
	path := filepath.Join(directory, "python.tar")
	writeTarArchive(t, path, []tar.Header{{Name: executablePath, Mode: 0o700, Size: int64(len(content)), Typeflag: tar.TypeReg}}, [][]byte{content})
	return path, int64(len(content)), executablePath
}

func writeTarArchive(t *testing.T, path string, headers []tar.Header, contents [][]byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	for index := range headers {
		if err := writer.WriteHeader(&headers[index]); err != nil {
			t.Fatal(err)
		}
		if len(contents[index]) > 0 {
			if _, err := writer.Write(contents[index]); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func runtimeExecutableSuffix() string {
	if filepath.Ext(os.Args[0]) == ".exe" {
		return ".exe"
	}
	return ""
}
