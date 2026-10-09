package vllm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	smokeModelDirectoryName = "smoke-model"
	maximumSmokeModelFiles  = 100000
	defaultSmokeTimeout     = 2 * time.Minute
)

func stageSmokeModel(ctx context.Context, profile Profile, artifacts map[string]string, environmentPath string, phase func(string) error) error {
	artifact, found := artifactByRole(profile, "smoke_model")
	if !found {
		return fmt.Errorf("vLLM profile requires exactly one signed smoke-model artifact")
	}
	archivePath := artifacts[artifact.Name]
	if archivePath == "" {
		return fmt.Errorf("signed smoke-model artifact is unavailable")
	}
	if err := phase("staging_smoke_model"); err != nil {
		return err
	}
	destination := filepath.Join(environmentPath, smokeModelDirectoryName)
	if err := ensurePrivateDirectory(destination); err != nil {
		return err
	}
	if err := extractSmokeModel(ctx, archivePath, destination, artifact); err != nil {
		return fmt.Errorf("extract signed smoke model: %w", err)
	}
	return nil
}

func extractSmokeModel(ctx context.Context, archivePath string, destination string, artifact Artifact) error {
	return extractAuthorizedArchive(ctx, archivePath, destination, artifact, "smoke-model")
}

func validatePortableInterpreterPath(runtimeDirectory string, interpreterPath string) error {
	if err := requirePathWithin(runtimeDirectory, interpreterPath); err != nil {
		return fmt.Errorf("validate isolated Python interpreter: %w", err)
	}
	info, err := os.Lstat(interpreterPath)
	if err != nil {
		return fmt.Errorf("validate isolated Python interpreter: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("isolated Python interpreter is not a regular file")
	}
	if err := makeExecutable(interpreterPath); err != nil {
		return fmt.Errorf("make isolated Python interpreter executable: %w", err)
	}
	return nil
}

func artifactByRole(profile Profile, role string) (Artifact, bool) {
	var selected Artifact
	found := false
	for _, artifact := range profile.Artifacts {
		if artifact.Role != role {
			continue
		}
		if found {
			return Artifact{}, false
		}
		selected = artifact
		found = true
	}
	return selected, found
}

func (tester CommandSmokeTester) testNativeServing(ctx context.Context, pythonPath string, environmentPath string, environment []string, logs io.Writer) error {
	socket, err := prepareSmokeSocket(environmentPath)
	if err != nil {
		return err
	}
	modelPath := filepath.Join(environmentPath, smokeModelDirectoryName)
	arguments := smokeServerArguments(socket.path, modelPath)
	return tester.launchAndProbeSmoke(ctx, pythonPath, arguments, environment, environmentPath, socket, logs)
}

func (tester CommandSmokeTester) testOCIServing(ctx context.Context, profile Profile, environmentPath string, enginePath string, engineName string, logs io.Writer) error {
	socket, err := prepareSmokeSocket(environmentPath)
	if err != nil {
		return err
	}
	modelPath := filepath.Join(environmentPath, smokeModelDirectoryName)
	if err := validateOCIMountPath(modelPath); err != nil {
		return fmt.Errorf("validate OCI smoke model: %w", err)
	}
	mounts := []ociMount{
		{Source: socket.directory, Destination: "/router-smoke"},
		{Source: modelPath, Destination: "/smoke-model", ReadOnly: true},
	}
	arguments := ociCommandArguments(engineName, profile, mounts, nil, smokeServerArguments("/router-smoke/vllm.sock", "/smoke-model"), false)
	return tester.launchAndProbeSmoke(ctx, enginePath, arguments, containerEngineEnvironment(environmentPath), environmentPath, socket, logs)
}

type smokeSocket struct {
	directory string
	path      string
}

func prepareSmokeSocket(environmentPath string) (smokeSocket, error) {
	if _, err := os.Lstat(environmentPath); err != nil {
		return smokeSocket{}, err
	}
	socketDirectory, err := os.MkdirTemp("", "tensor-router-vllm-smoke-")
	if err != nil {
		return smokeSocket{}, err
	}
	if err := os.Chmod(socketDirectory, 0o700); err != nil {
		_ = os.Remove(socketDirectory)
		return smokeSocket{}, err
	}
	return smokeSocket{directory: socketDirectory, path: filepath.Join(socketDirectory, "vllm.sock")}, nil
}

func (socket smokeSocket) remove() {
	_ = os.Remove(socket.path)
	_ = os.Remove(socket.directory)
}

func smokeServerArguments(socketPath string, modelPath string) []string {
	return append(serveCommand(modelPath, socketPath),
		"--served-model-name", "tensor-router-vllm-smoke",
		"--max-num-seqs", "1",
		"--enforce-eager",
	)
}

func (tester CommandSmokeTester) launchAndProbeSmoke(ctx context.Context, executable string, arguments []string, environment []string, directory string, socket smokeSocket, logs io.Writer) error {
	launcher := tester.Launcher
	if launcher == nil {
		launcher = ExecRuntimeLauncher{}
	}
	timeout := tester.ServingTimeout
	if timeout <= 0 {
		timeout = defaultSmokeTimeout
	}
	smokeContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	child, err := launcher.Start(smokeContext, executable, arguments, environment, directory, logs)
	if err != nil {
		return fmt.Errorf("start vLLM serving smoke: %w", err)
	}
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()
	probeError := probeSmokeServer(smokeContext, socket.path, exited)
	stopContext, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	stopError := child.Stop(stopContext)
	stopCancel()
	socket.remove()
	if probeError != nil {
		return probeError
	}
	if stopError != nil && !errors.Is(stopError, os.ErrProcessDone) {
		return fmt.Errorf("stop vLLM serving smoke: %w", stopError)
	}
	return nil
}

func probeSmokeServer(ctx context.Context, socketPath string, exited <-chan error) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	transport := &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		DialContext: func(dialContext context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: time.Second}).DialContext(dialContext, "unix", socketPath)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	for {
		select {
		case err := <-exited:
			if err == nil {
				return fmt.Errorf("vLLM smoke server exited before health check")
			}
			return fmt.Errorf("vLLM smoke server exited before health check: %w", err)
		case <-ctx.Done():
			return fmt.Errorf("vLLM smoke server health check: %w", ctx.Err())
		case <-ticker.C:
		}
		if validatePrivateSocket(socketPath) != nil {
			continue
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://vllm-smoke.local/health", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		_ = response.Body.Close()
		if response.StatusCode == http.StatusOK {
			return nil
		}
	}
}
