package kobold

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"tensors-router/internal/mcp"
)

const reloadHealthyTimeout = 90 * time.Second

func (manager *Manager) ReloadConfig(ctx context.Context, filename string) error {
	runtimeFilename, generatedPath, _, err := manager.runtimeConfig(filename)
	if err != nil {
		return err
	}
	if manager.role == embeddingsRole {
		if err := manager.ensureEmbeddingsProcess(ctx, generatedPath); err != nil {
			return err
		}
	}
	body, err := manager.adminReloadBody(filename, runtimeFilename)
	if err != nil {
		return err
	}
	if err := manager.postAdminReload(ctx, body, generatedPath); err != nil {
		return err
	}
	if err := manager.lifecycle.Lock(ctx); err != nil {
		manager.removeGenerated(generatedPath)
		return err
	}
	exitDone := manager.exitDone
	manager.lifecycle.Unlock()
	if err := manager.waitHealthy(ctx, reloadHealthyTimeout, exitDone); err != nil {
		manager.removeGenerated(generatedPath)
		return err
	}
	return nil
}

func (manager *Manager) ensureEmbeddingsProcess(ctx context.Context, generatedPath string) error {
	if err := manager.lifecycle.Lock(ctx); err != nil {
		return err
	}
	if manager.cmd != nil && manager.cmd.Process != nil {
		manager.lifecycle.Unlock()
		return nil
	}
	err := manager.startLocked(ctx)
	manager.lifecycle.Unlock()
	if err != nil {
		manager.removeGenerated(generatedPath)
	}
	return err
}

func (manager *Manager) adminReloadBody(filename string, runtimeFilename string) ([]byte, error) {
	configFilename := runtimeFilename
	baseConfig := ""
	if manager.config.MCP != nil {
		result, err := manager.config.MCP.Reconcile(filename, mcp.BackendKobold)
		if err != nil {
			return nil, err
		}
		if result.Enabled {
			configFilename = filepath.Join(".router-mcp", filename)
			baseConfig = runtimeFilename
		}
	}
	return json.Marshal(map[string]string{
		"filename":       configFilename,
		"baseconfig":     baseConfig,
		"overrideconfig": "",
	})
}

func (manager *Manager) postAdminReload(ctx context.Context, body []byte, generatedPath string) error {
	target := manager.URL()
	target.Path = "/api/admin/reload_config"
	reloadContext, cancelReload := context.WithTimeout(ctx, adminReloadTimeout)
	defer cancelReload()
	request, err := http.NewRequestWithContext(reloadContext, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+manager.adminPassword)

	response, err := manager.reloadClient.Do(request)
	if err != nil {
		manager.removeGenerated(generatedPath)
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if err := adminReloadFailure(response.StatusCode, responseBody); err != nil {
		manager.removeGenerated(generatedPath)
		return err
	}
	return nil
}

func adminReloadFailure(status int, responseBody []byte) error {
	if status < 200 || status > 299 {
		return fmt.Errorf("admin reload failed with status %d: %s", status, reloadErrorDetail(responseBody))
	}
	var reload reloadResponse
	if err := json.Unmarshal(responseBody, &reload); err != nil {
		return err
	}
	if reload.Success {
		return nil
	}
	if reload.Error != "" {
		return fmt.Errorf("admin reload failed: %s", reload.Error)
	}
	// KoboldCpp can report failure with no error field at all. Fall back to the raw
	// response rather than an unattributable "admin reload failed".
	return fmt.Errorf("admin reload failed: %s", reloadErrorDetail(responseBody))
}
