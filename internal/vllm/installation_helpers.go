package vllm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func artifactPathByRole(profile Profile, artifacts map[string]string, role string) (string, error) {
	paths := artifactPathsByRole(profile, artifacts, role)
	if len(paths) != 1 {
		return "", fmt.Errorf("vLLM profile requires exactly one %s artifact", role)
	}
	return paths[0], nil
}

func artifactPathsByRole(profile Profile, artifacts map[string]string, roles ...string) []string {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	paths := make([]string, 0)
	for _, artifact := range profile.Artifacts {
		if _, accepted := allowed[artifact.Role]; !accepted {
			continue
		}
		if path := artifacts[artifact.Name]; path != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func artifactDirectories(paths []string) []string {
	directories := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		directory := filepath.Dir(path)
		if _, exists := seen[directory]; exists {
			continue
		}
		seen[directory] = struct{}{}
		directories = append(directories, directory)
	}
	sort.Strings(directories)
	return directories
}

func makeExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	return os.Chmod(path, info.Mode().Perm()|0o500)
}

func copyRegularFile(ctx context.Context, sourcePath string, destinationPath string, mode os.FileMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sourceInfo, err := os.Lstat(sourcePath)
	if err != nil {
		return err
	}
	if !sourceInfo.Mode().IsRegular() || sourceInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("source is not a regular file")
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	failed := true
	defer func() {
		_ = destination.Close()
		if failed {
			_ = os.Remove(destinationPath)
		}
	}()
	if _, err := copyContext(ctx, destination, source); err != nil {
		return err
	}
	if err := destination.Sync(); err != nil {
		return err
	}
	if err := destination.Close(); err != nil {
		return err
	}
	failed = false
	return nil
}

func copyContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	buffer := make([]byte, 256<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		count, readError := source.Read(buffer)
		if count > 0 {
			written, writeError := destination.Write(buffer[:count])
			total += int64(written)
			if writeError != nil {
				return total, writeError
			}
			if written != count {
				return total, io.ErrShortWrite
			}
		}
		if readError != nil {
			if errors.Is(readError, io.EOF) {
				return total, nil
			}
			return total, readError
		}
	}
}

func environmentPythonPath(environmentPath string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(environmentPath, "Scripts", "python.exe")
	}
	return filepath.Join(environmentPath, "bin", "python")
}

func isolatedInstallEnvironment(environmentPath string) []string {
	environment := []string{
		"HOME=" + environmentPath,
		"UV_CACHE_DIR=" + filepath.Join(environmentPath, "uv-cache"),
		"UV_NO_CONFIG=1",
		"UV_OFFLINE=1",
		"PYTHONNOUSERSITE=1",
		"PYTHONDONTWRITEBYTECODE=1",
		"PIP_CONFIG_FILE=" + os.DevNull,
		"PIP_DISABLE_PIP_VERSION_CHECK=1",
		"PIP_NO_INDEX=1",
	}
	if path := os.Getenv("PATH"); path != "" && !strings.ContainsAny(path, "\x00\r\n") {
		environment = append(environment, "PATH="+path)
	}
	return environment
}

func findExecutable(name string) (string, error) {
	return exec.LookPath(name)
}
