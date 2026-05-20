package kobold

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type ProcessConfig struct {
	BackendURL   string
	BinaryPath   string
	ConfigDir    string
	DataDir      string
	ExtraArgs    []string
	Multiuser    int
	Quiet        bool
	SkipLauncher bool
	NoModel      bool
	HideWindow   bool
	Logging      bool
}

type Manager struct {
	config        ProcessConfig
	backendURL    *url.URL
	adminPassword string
	client        *http.Client
	mu            sync.Mutex
	cmd           *exec.Cmd
	logFile       *os.File
	waitDone      chan error
}

type reloadResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func NewManager(config ProcessConfig) (*Manager, error) {
	backendURL, err := url.Parse(config.BackendURL)
	if err != nil {
		return nil, err
	}
	if config.Multiuser < 1 {
		config.Multiuser = 1
	}

	adminPassword, err := generateAdminPassword()
	if err != nil {
		return nil, err
	}

	return &Manager{
		config:        config,
		backendURL:    backendURL,
		adminPassword: adminPassword,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (manager *Manager) URL() *url.URL {
	copyValue := *manager.backendURL
	return &copyValue
}

func (manager *Manager) AdminPassword() string {
	return manager.adminPassword
}

func (manager *Manager) LaunchArguments() []string {
	host, port := hostPort(manager.backendURL)
	args := []string{
		"--host", host,
		"--port", port,
		"--admin",
		"--adminpassword", manager.adminPassword,
		"--admindir", manager.config.ConfigDir,
		"--routermode",
		"--multiuser", strconv.Itoa(manager.config.Multiuser),
	}
	if manager.config.NoModel {
		args = append(args, "--nomodel")
	}
	if manager.config.SkipLauncher {
		args = append(args, "--skiplauncher")
	}
	if manager.config.Quiet {
		args = append(args, "--quiet")
	}
	args = append(args, manager.config.ExtraArgs...)
	return args
}

func (manager *Manager) Start(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if manager.cmd != nil && manager.cmd.Process != nil && manager.Healthy(ctx) {
		return nil
	}

	if err := os.MkdirAll(manager.config.DataDir, 0o755); err != nil {
		return err
	}

	var logFile *os.File
	processOutput := io.Writer(io.Discard)
	if manager.config.Logging {
		logPath := filepath.Join(manager.config.DataDir, "koboldcpp.log")
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		processOutput = logFile
	}

	cmd := exec.Command(manager.config.BinaryPath, manager.LaunchArguments()...)
	prepareCommand(cmd, manager.config)
	cmd.Stdout = processOutput
	cmd.Stderr = processOutput

	if err := cmd.Start(); err != nil {
		_ = closeLogFile(logFile)
		return err
	}

	manager.cmd = cmd
	manager.logFile = logFile
	waitDone := make(chan error, 1)
	manager.waitDone = waitDone

	go func() {
		waitDone <- cmd.Wait()
		_ = closeLogFile(logFile)
	}()

	if err := manager.waitHealthy(ctx, 90*time.Second); err != nil {
		_ = killCommand(cmd)
		manager.cmd = nil
		manager.logFile = nil
		manager.waitDone = nil
		return err
	}

	return nil
}

func (manager *Manager) Stop(ctx context.Context) error {
	manager.mu.Lock()
	cmd := manager.cmd
	manager.cmd = nil
	logFile := manager.logFile
	manager.logFile = nil
	waitDone := manager.waitDone
	manager.waitDone = nil
	manager.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		if logFile != nil {
			return closeLogFile(logFile)
		}
		return nil
	}

	_ = terminateCommand(cmd)

	select {
	case <-ctx.Done():
		_ = killCommand(cmd)
		return ctx.Err()
	case <-time.After(10 * time.Second):
		_ = killCommand(cmd)
		select {
		case <-waitDone:
		case <-time.After(5 * time.Second):
		}
	case <-waitDone:
	}

	return nil
}

func closeLogFile(logFile *os.File) error {
	if logFile == nil {
		return nil
	}
	return logFile.Close()
}

func (manager *Manager) Restart(ctx context.Context) error {
	if err := manager.Stop(ctx); err != nil {
		return err
	}
	if err := manager.Start(ctx); err != nil {
		return err
	}
	return nil
}

func (manager *Manager) ReloadConfig(ctx context.Context, filename string) error {
	body, err := json.Marshal(map[string]string{
		"filename":       filename,
		"overrideconfig": "",
	})
	if err != nil {
		return err
	}

	target := manager.URL()
	target.Path = "/api/admin/reload_config"

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+manager.adminPassword)

	response, err := manager.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("admin reload failed with status %d", response.StatusCode)
	}

	var reload reloadResponse
	if err := json.Unmarshal(responseBody, &reload); err != nil {
		return err
	}
	if !reload.Success {
		if reload.Error != "" {
			return fmt.Errorf("admin reload failed: %s", reload.Error)
		}
		return fmt.Errorf("admin reload failed")
	}

	return manager.waitHealthy(ctx, 90*time.Second)
}

func (manager *Manager) Healthy(ctx context.Context) bool {
	target := manager.URL()
	target.Path = "/api/extra/version"

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return false
	}

	response, err := manager.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	return response.StatusCode >= 200 && response.StatusCode < 500
}

func (manager *Manager) waitHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if manager.Healthy(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("koboldcpp did not become healthy within %s", timeout)
}

func generateAdminPassword() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func hostPort(backendURL *url.URL) (string, string) {
	host := backendURL.Hostname()
	port := backendURL.Port()
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		switch backendURL.Scheme {
		case "https":
			port = "443"
		default:
			port = "5001"
		}
	}
	if parsed := net.ParseIP(host); parsed != nil && parsed.IsUnspecified() {
		host = "127.0.0.1"
	}
	return host, port
}
