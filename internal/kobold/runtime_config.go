package kobold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const routerRuntimeDirectory = ".router-runtime"

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
	manager.state.Lock()
	defer manager.state.Unlock()
	_ = os.Remove(path)
	delete(manager.generated, path)
}

func (manager *Manager) cleanupGeneratedLocked() error {
	manager.state.Lock()
	defer manager.state.Unlock()
	var firstErr error
	for path := range manager.generated {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
		delete(manager.generated, path)
	}
	if manager.config.ConfigDir != "" {
		_ = os.Remove(filepath.Join(manager.config.ConfigDir, routerRuntimeDirectory))
	}
	return firstErr
}
