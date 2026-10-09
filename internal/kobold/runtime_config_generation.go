package kobold

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"tensors-router/internal/catalog"
)

var primaryOnlyEmbeddingKeys = []string{"embeddingsmodel", "embeddingsmaxctx", "embeddingsgpu", "run_embed_separate"}

func (manager *Manager) runtimeConfig(filename string) (string, string, catalog.RuntimeConfig, error) {
	if filename == "" || filename != filepath.Base(filename) {
		return "", "", catalog.RuntimeConfig{}, fmt.Errorf("config filename %q is invalid", filename)
	}
	sourcePath := filepath.Join(manager.config.ConfigDir, filename)
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		if manager.role != embeddingsRole && os.IsNotExist(err) {
			return filename, "", catalog.RuntimeConfig{}, nil
		}
		return "", "", catalog.RuntimeConfig{}, err
	}
	metadata, err := catalog.LoadRuntimeConfig(sourcePath)
	if err != nil {
		return "", "", catalog.RuntimeConfig{}, err
	}
	if !metadata.RunEmbedSeparate {
		if manager.role == embeddingsRole {
			return "", "", catalog.RuntimeConfig{}, fmt.Errorf("config %q does not enable run_embed_separate", filename)
		}
		return filename, "", metadata, nil
	}
	generatedContent, err := manager.roleRuntimeContent(content, metadata)
	if err != nil {
		return "", "", catalog.RuntimeConfig{}, err
	}
	runtimeDir := filepath.Join(manager.config.ConfigDir, routerRuntimeDirectory)
	if err := ensurePrivateRuntimeDir(runtimeDir); err != nil {
		return "", "", catalog.RuntimeConfig{}, err
	}
	runtimeName := manager.runtimeConfigName(filename, generatedContent)
	runtimePath := filepath.Join(runtimeDir, runtimeName)
	if err := materializeRuntimeConfig(runtimeDir, runtimePath, generatedContent); err != nil {
		return "", "", catalog.RuntimeConfig{}, err
	}
	manager.state.Lock()
	manager.generated[runtimePath] = struct{}{}
	manager.state.Unlock()
	return filepath.Join(routerRuntimeDirectory, runtimeName), runtimePath, metadata, nil
}

func (manager *Manager) roleRuntimeContent(content []byte, metadata catalog.RuntimeConfig) ([]byte, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(content, &values); err != nil {
		return nil, err
	}
	if manager.role == embeddingsRole {
		manager.state.Lock()
		if metadata.EmbeddingsGPU {
			manager.roleArgs = nil
		} else {
			manager.roleArgs = []string{"--usecpu"}
		}
		manager.state.Unlock()
		values = embeddingRuntimeValues(values, metadata.EmbeddingsGPU)
	} else {
		for _, key := range primaryOnlyEmbeddingKeys {
			delete(values, key)
		}
	}
	return json.Marshal(values)
}

func (manager *Manager) runtimeConfigName(filename string, generatedContent []byte) string {
	digest := sha256.Sum256(append([]byte(manager.role+"\x00"), generatedContent...))
	roleName := "primary"
	if manager.role == embeddingsRole {
		roleName = embeddingsRole
	}
	return fmt.Sprintf("%s-%s-%x.kcpps", safeRuntimeStem(filename), roleName, digest[:8])
}

func materializeRuntimeConfig(runtimeDir string, runtimePath string, generatedContent []byte) error {
	info, err := os.Lstat(runtimePath)
	if os.IsNotExist(err) {
		return writeRuntimeConfigAtomically(runtimeDir, runtimePath, generatedContent)
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("generated runtime config %q is not a regular file", runtimePath)
	}
	existingContent, err := os.ReadFile(runtimePath)
	if err != nil {
		return err
	}
	if !bytes.Equal(existingContent, generatedContent) {
		return fmt.Errorf("generated runtime config %q has unexpected content", runtimePath)
	}
	return os.Chmod(runtimePath, 0o600)
}

func writeRuntimeConfigAtomically(runtimeDir string, runtimePath string, generatedContent []byte) error {
	temporary, err := os.CreateTemp(runtimeDir, ".runtime-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	writeErr := temporary.Chmod(0o600)
	if writeErr == nil {
		_, writeErr = temporary.Write(generatedContent)
	}
	if closeErr := temporary.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = os.Rename(temporaryPath, runtimePath)
	}
	if writeErr != nil {
		_ = os.Remove(temporaryPath)
	}
	return writeErr
}
