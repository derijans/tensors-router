package vllm

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

func extractAuthorizedArchive(ctx context.Context, archivePath string, destination string, artifact Artifact, artifactKind string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	archiveInfo, err := os.Lstat(archivePath)
	if err != nil {
		return err
	}
	if !archiveInfo.Mode().IsRegular() || archiveInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s archive is not a regular file", artifactKind)
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()
	var source io.Reader = archive
	if artifact.ArchiveFormat == "tar.gz" {
		compressed, err := gzip.NewReader(archive)
		if err != nil {
			return err
		}
		defer compressed.Close()
		source = compressed
	}
	reader := tar.NewReader(source)
	seen := make(map[string]struct{})
	var unpacked int64
	var files int
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		normalized, err := normalizePortableArchivePath(header.Name)
		if err != nil {
			return err
		}
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("%s archive contains duplicate path %q", artifactKind, normalized)
		}
		seen[normalized] = struct{}{}
		files++
		if files > maximumSmokeModelFiles {
			return fmt.Errorf("%s archive contains too many entries", artifactKind)
		}
		target := filepath.Join(destination, filepath.FromSlash(normalized))
		if err := requirePathWithin(destination, target); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := ensurePrivateDirectory(target); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > artifact.UnpackedSize-unpacked {
				return fmt.Errorf("%s archive exceeds authorized unpacked size", artifactKind)
			}
			if err := ensurePrivateDirectory(filepath.Dir(target)); err != nil {
				return err
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			written, copyError := copyContext(ctx, file, io.LimitReader(reader, header.Size+1))
			closeError := file.Close()
			if copyError != nil {
				return copyError
			}
			if closeError != nil {
				return closeError
			}
			if written != header.Size {
				return fmt.Errorf("%s archive entry %q has invalid size", artifactKind, normalized)
			}
			unpacked += written
		default:
			return fmt.Errorf("%s archive contains unsupported entry %q", artifactKind, normalized)
		}
	}
	if unpacked != artifact.UnpackedSize || files == 0 {
		return fmt.Errorf("%s archive unpacked size %d does not match authorized size %d", artifactKind, unpacked, artifact.UnpackedSize)
	}
	return nil
}

func normalizePortableArchivePath(value string) (string, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || strings.HasPrefix(value, "/") || strings.ContainsAny(value, ":\x00") {
		return "", fmt.Errorf("archive path is invalid")
	}
	normalized := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") || normalized != value && normalized+"/" != value || filepath.IsAbs(filepath.FromSlash(value)) {
		return "", fmt.Errorf("archive path %q is unsafe", value)
	}
	return normalized, nil
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
	socketDirectory, socketPath, err := prepareSmokeSocketDirectory(environmentPath)
	if err != nil {
		return err
	}
	modelPath := filepath.Join(environmentPath, smokeModelDirectoryName)
	arguments := smokeServerArguments(socketPath, modelPath)
	return tester.launchAndProbeSmoke(ctx, pythonPath, arguments, environment, environmentPath, socketDirectory, socketPath, logs)
}

func (tester CommandSmokeTester) testOCIServing(ctx context.Context, profile Profile, environmentPath string, enginePath string, engineName string, logs io.Writer) error {
	socketDirectory, socketPath, err := prepareSmokeSocketDirectory(environmentPath)
	if err != nil {
		return err
	}
	modelPath := filepath.Join(environmentPath, smokeModelDirectoryName)
	if err := validateOCIMountPath(modelPath); err != nil {
		return fmt.Errorf("validate OCI smoke model: %w", err)
	}
	mounts := []ociMount{
		{Source: socketDirectory, Destination: "/router-smoke"},
		{Source: modelPath, Destination: "/smoke-model", ReadOnly: true},
	}
	arguments := ociCommandArguments(engineName, profile, mounts, nil, smokeServerArguments("/router-smoke/vllm.sock", "/smoke-model"), false)
	return tester.launchAndProbeSmoke(ctx, enginePath, arguments, containerEngineEnvironment(environmentPath), environmentPath, socketDirectory, socketPath, logs)
}

func prepareSmokeSocketDirectory(environmentPath string) (string, string, error) {
	if _, err := os.Lstat(environmentPath); err != nil {
		return "", "", err
	}
	socketDirectory, err := os.MkdirTemp("", "tensor-router-vllm-smoke-")
	if err != nil {
		return "", "", err
	}
	if err := os.Chmod(socketDirectory, 0o700); err != nil {
		_ = os.Remove(socketDirectory)
		return "", "", err
	}
	socketPath := filepath.Join(socketDirectory, "vllm.sock")
	return socketDirectory, socketPath, nil
}

func smokeServerArguments(socketPath string, modelPath string) []string {
	return []string{
		"-I", "-m", "vllm.entrypoints.openai.api_server",
		"--uds", socketPath,
		"--model", modelPath,
		"--served-model-name", "tensor-router-vllm-smoke",
		"--max-num-seqs", "1",
		"--enforce-eager",
	}
}

func (tester CommandSmokeTester) launchAndProbeSmoke(ctx context.Context, executable string, arguments []string, environment []string, directory string, socketDirectory string, socketPath string, logs io.Writer) error {
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
	probeError := probeSmokeServer(smokeContext, socketPath, exited)
	stopContext, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
	stopError := child.Stop(stopContext)
	stopCancel()
	_ = os.Remove(socketPath)
	_ = os.Remove(socketDirectory)
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
