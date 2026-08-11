package kobold

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tensors-router/internal/catalog"
)

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
	var values map[string]json.RawMessage
	if err := json.Unmarshal(content, &values); err != nil {
		return "", "", catalog.RuntimeConfig{}, err
	}
	if manager.role == embeddingsRole {
		manager.mu.Lock()
		if metadata.EmbeddingsGPU {
			manager.roleArgs = nil
		} else {
			manager.roleArgs = []string{"--usecpu"}
		}
		manager.mu.Unlock()
		values = embeddingRuntimeValues(values, metadata.EmbeddingsGPU)
	} else {
		delete(values, "embeddingsmodel")
		delete(values, "embeddingsmaxctx")
		delete(values, "embeddingsgpu")
		delete(values, "run_embed_separate")
	}
	generatedContent, err := json.Marshal(values)
	if err != nil {
		return "", "", catalog.RuntimeConfig{}, err
	}
	runtimeDir := filepath.Join(manager.config.ConfigDir, ".router-runtime")
	if err := ensurePrivateRuntimeDir(runtimeDir); err != nil {
		return "", "", catalog.RuntimeConfig{}, err
	}
	digest := sha256.Sum256(append([]byte(manager.role+"\x00"), generatedContent...))
	roleName := "primary"
	if manager.role == embeddingsRole {
		roleName = embeddingsRole
	}
	runtimeName := fmt.Sprintf("%s-%s-%x.kcpps", safeRuntimeStem(filename), roleName, digest[:8])
	runtimePath := filepath.Join(runtimeDir, runtimeName)
	if info, err := os.Lstat(runtimePath); os.IsNotExist(err) {
		temporary, createErr := os.CreateTemp(runtimeDir, ".runtime-*.tmp")
		if createErr != nil {
			return "", "", catalog.RuntimeConfig{}, createErr
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
			return "", "", catalog.RuntimeConfig{}, writeErr
		}
	} else if err != nil {
		return "", "", catalog.RuntimeConfig{}, err
	} else {
		if !info.Mode().IsRegular() {
			return "", "", catalog.RuntimeConfig{}, fmt.Errorf("generated runtime config %q is not a regular file", runtimePath)
		}
		existingContent, readErr := os.ReadFile(runtimePath)
		if readErr != nil {
			return "", "", catalog.RuntimeConfig{}, readErr
		}
		if !bytes.Equal(existingContent, generatedContent) {
			return "", "", catalog.RuntimeConfig{}, fmt.Errorf("generated runtime config %q has unexpected content", runtimePath)
		}
		if chmodErr := os.Chmod(runtimePath, 0o600); chmodErr != nil {
			return "", "", catalog.RuntimeConfig{}, chmodErr
		}
	}
	manager.mu.Lock()
	manager.generated[runtimePath] = struct{}{}
	manager.mu.Unlock()
	return filepath.Join(".router-runtime", runtimeName), runtimePath, metadata, nil
}

func embeddingRuntimeValues(source map[string]json.RawMessage, gpu bool) map[string]json.RawMessage {
	shared := map[string]bool{
		"embeddingsmodel": true, "embeddingsmaxctx": true, "embeddingsgpu": true,
		"threads": true, "blasthreads": true,
		"batchsize": true, "ubatchsize": true, "gpulayers": true, "splitmode": true,
		"tensor_split": true, "maingpu": true, "usecuda": true, "usecublas": true,
		"usevulkan": true, "usecpu": true, "flashattention": true, "noflashattention": true,
		"usemmap": true, "usemlock": true, "load_mode": true,
	}
	result := make(map[string]json.RawMessage)
	for key, value := range source {
		if shared[key] {
			result[key] = append(json.RawMessage(nil), value...)
		}
	}
	if gpu {
		result["embeddingsgpu"] = json.RawMessage("true")
		delete(result, "usecpu")
	} else {
		result["embeddingsgpu"] = json.RawMessage("false")
		result["usecpu"] = json.RawMessage("true")
		result["gpulayers"] = json.RawMessage("0")
		delete(result, "usecuda")
		delete(result, "usecublas")
		delete(result, "usevulkan")
		delete(result, "tensor_split")
		delete(result, "maingpu")
	}
	return result
}

func ensurePrivateRuntimeDir(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("runtime config directory %q is not a private directory", path)
	}
	return os.Chmod(path, 0o700)
}

func safeRuntimeStem(filename string) string {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	var safe strings.Builder
	for _, character := range stem {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' || character == '_' || character == '.' {
			safe.WriteRune(character)
		} else {
			safe.WriteByte('_')
		}
		if safe.Len() >= 64 {
			break
		}
	}
	if safe.Len() == 0 {
		return "config"
	}
	return safe.String()
}

func (manager *Manager) removeGenerated(path string) {
	if path == "" {
		return
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	_ = os.Remove(path)
	delete(manager.generated, path)
}

func (manager *Manager) cleanupGeneratedLocked() error {
	var firstErr error
	for path := range manager.generated {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
		delete(manager.generated, path)
	}
	if manager.config.ConfigDir != "" {
		_ = os.Remove(filepath.Join(manager.config.ConfigDir, ".router-runtime"))
	}
	return firstErr
}
