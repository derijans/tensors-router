package kobold

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/backenddiagnostic"
	"tensors-router/internal/backendendpoint"
	"tensors-router/internal/loadcapture"
	"tensors-router/internal/mcp"
	"tensors-router/internal/portalloc"
	"tensors-router/internal/processcontrol"
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
	MCP          *mcp.Reconciler
}

type Manager struct {
	config        ProcessConfig
	endpoint      *backendendpoint.Endpoint
	adminPassword string
	client        *http.Client
	mu            sync.Mutex
	exitMu        sync.RWMutex
	cmd           *exec.Cmd
	logFile       *os.File
	waitDone      chan error
	exitDone      <-chan error
	exitErr       error
	exitObserved  bool
	capture       *backenddiagnostic.Capture
	captureHub    *loadcapture.Hub
	forceNoModel  bool
	role          string
	roleArgs      []string
	generated     map[string]struct{}
}

const embeddingsRole = "embeddings"

type reloadResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

func NewManager(config ProcessConfig) (*Manager, error) {
	return newManager(config, "")
}

func NewEmbeddingsManager(config ProcessConfig) (*Manager, error) {
	return newManager(config, embeddingsRole)
}

func newManager(config ProcessConfig, role string) (*Manager, error) {
	endpoint, err := backendendpoint.NewEndpoint(config.BackendURL)
	if err != nil {
		return nil, err
	}
	if err := backendendpoint.RejectConflictingArgs(config.ExtraArgs, "--host", "--port"); err != nil {
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
		endpoint:      endpoint,
		adminPassword: adminPassword,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		capture:    backenddiagnostic.NewCapture(adminPassword),
		captureHub: loadcapture.NewHub(),
		role:       role,
		generated:  make(map[string]struct{}),
	}, nil
}

func (manager *Manager) URL() *url.URL {
	return manager.endpoint.URL()
}

func (manager *Manager) AdminPassword() string {
	return manager.adminPassword
}

func (manager *Manager) LaunchArguments() []string {
	host, port := manager.endpoint.HostPort()
	args := []string{
		"--host", host,
		"--port", port,
		"--admin",
		"--adminpassword", manager.adminPassword,
		"--admindir", manager.config.ConfigDir,
		"--multiuser", strconv.Itoa(manager.config.Multiuser),
	}
	if manager.config.NoModel || manager.forceNoModel || manager.role == embeddingsRole {
		args = append(args, "--nomodel")
	}
	if manager.config.SkipLauncher {
		args = append(args, "--skiplauncher")
	}
	if manager.config.Quiet {
		args = append(args, "--quiet")
	}
	extraArgs := launchExtraArgs(manager.config.ExtraArgs)
	if manager.role == embeddingsRole {
		extraArgs = embeddingLaunchExtraArgs(extraArgs, len(manager.roleArgs) > 0)
	}
	args = append(args, extraArgs...)
	args = append(args, manager.roleArgs...)
	return args
}

func launchExtraArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--routermode" {
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}

func embeddingLaunchExtraArgs(args []string, cpu bool) []string {
	owned := map[string]bool{"--usecpu": true}
	if cpu {
		owned["--usecuda"] = true
		owned["--usecublas"] = true
		owned["--usevulkan"] = true
		owned["--gpulayers"] = true
		owned["--tensor_split"] = true
		owned["--maingpu"] = true
	}
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		key := argument
		if separator := strings.IndexByte(key, '='); separator >= 0 {
			key = key[:separator]
		}
		if !owned[key] {
			filtered = append(filtered, argument)
			continue
		}
		if argument == key && index+1 < len(args) && !strings.HasPrefix(args[index+1], "--") {
			index++
		}
	}
	return filtered
}

func (manager *Manager) Start(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	return manager.startLocked(ctx)
}

const maxPortAttempts = 3

func (manager *Manager) startLocked(ctx context.Context) error {
	if manager.cmd != nil && manager.cmd.Process != nil && manager.Healthy(ctx) {
		return nil
	}
	if manager.cmd != nil {
		if err := manager.stopLocked(ctx); err != nil {
			return err
		}
	}

	if err := os.MkdirAll(manager.config.DataDir, 0o755); err != nil {
		return err
	}

	for attempt := 1; ; attempt++ {
		if err := manager.endpoint.Reserve(portalloc.Default()); err != nil {
			return err
		}
		host, port := manager.endpoint.HostPort()
		if err := portalloc.CheckAvailable(host, port); err != nil {
			return err
		}
		err := manager.spawnLocked(ctx)
		if err == nil {
			return nil
		}
		var exitErr *backendExitedError
		if attempt >= maxPortAttempts || !manager.endpoint.Dynamic() || !errors.As(err, &exitErr) {
			return err
		}
		// The child exited during startup on a dynamically allocated port,
		// which is the signature of a lost race for that port (something
		// else bound it between our probe and the child's own bind). Free
		// the port and try again with a freshly reserved one.
		manager.endpoint.Release(portalloc.Default())
	}
}

func (manager *Manager) spawnLocked(ctx context.Context) error {
	var logFile *os.File
	stdout := io.MultiWriter(manager.capture, manager.captureHub.Stdout())
	stderr := io.MultiWriter(manager.capture, manager.captureHub.Stderr())
	if manager.config.Logging {
		logName := "koboldcpp.log"
		if manager.role == embeddingsRole {
			logName = "koboldcpp-embeddings.log"
		}
		logPath := filepath.Join(manager.config.DataDir, logName)
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		stdout = io.MultiWriter(stdout, logFile)
		stderr = io.MultiWriter(stderr, logFile)
	}

	cmd := exec.Command(manager.config.BinaryPath, manager.LaunchArguments()...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := processcontrol.Start(cmd, processcontrol.Options{HideWindow: manager.config.HideWindow, ParentDeathGracePeriod: 10 * time.Second}); err != nil {
		_ = closeLogFile(logFile)
		return err
	}

	manager.cmd = cmd
	manager.exitMu.Lock()
	manager.exitErr = nil
	manager.exitObserved = false
	manager.exitMu.Unlock()
	manager.logFile = logFile
	waitDone := make(chan error, 1)
	manager.waitDone = waitDone
	exitDone := make(chan error, 1)
	manager.exitDone = exitDone

	go func() {
		err := cmd.Wait()
		manager.exitMu.Lock()
		manager.exitErr = err
		manager.exitObserved = true
		manager.exitMu.Unlock()
		manager.capture.RecordExit(err)
		waitDone <- err
		exitDone <- err
		_ = closeLogFile(logFile)
	}()

	if err := manager.waitHealthy(ctx, 90*time.Second, exitDone); err != nil {
		_ = processcontrol.Kill(cmd)
		manager.cmd = nil
		manager.logFile = nil
		manager.waitDone = nil
		manager.exitDone = nil
		return err
	}

	return nil
}

func (manager *Manager) Stop(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	err := manager.stopLocked(ctx)
	cleanupErr := manager.cleanupGeneratedLocked()
	if err != nil {
		return err
	}
	return cleanupErr
}

// ReleaseEndpoint returns a dynamically allocated endpoint's port to the
// shared allocator. Call this only when the manager itself is being torn
// down for good (router shutdown) — Stop alone (used by Restart and Unload)
// deliberately keeps a dynamic endpoint's port reserved so those operations
// do not change the backend's address. It is a no-op for a pinned endpoint.
func (manager *Manager) ReleaseEndpoint() {
	manager.endpoint.Release(portalloc.Default())
}

func (manager *Manager) stopLocked(ctx context.Context) error {
	cmd := manager.cmd
	manager.cmd = nil
	logFile := manager.logFile
	manager.logFile = nil
	waitDone := manager.waitDone
	manager.waitDone = nil
	manager.exitDone = nil

	if cmd == nil || cmd.Process == nil {
		if logFile != nil {
			return closeLogFile(logFile)
		}
		return nil
	}

	return stopManagedProcess(ctx, cmd, waitDone)
}

func stopManagedProcess(ctx context.Context, cmd *exec.Cmd, waitDone <-chan error) error {
	return processcontrol.Stop(ctx, cmd, waitDone, 10*time.Second, 5*time.Second)
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

func (manager *Manager) Unload(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if err := manager.stopLocked(ctx); err != nil {
		return err
	}
	if manager.role == embeddingsRole {
		return manager.cleanupGeneratedLocked()
	}
	manager.forceNoModel = true
	err := manager.startLocked(ctx)
	manager.forceNoModel = false
	if err != nil {
		return err
	}
	return manager.cleanupGeneratedLocked()
}

func (manager *Manager) ReloadConfig(ctx context.Context, filename string) error {
	runtimeFilename, generatedPath, _, err := manager.runtimeConfig(filename)
	if err != nil {
		return err
	}
	if manager.role == embeddingsRole {
		manager.mu.Lock()
		if manager.cmd == nil || manager.cmd.Process == nil {
			if err := manager.startLocked(ctx); err != nil {
				manager.mu.Unlock()
				manager.removeGenerated(generatedPath)
				return err
			}
		}
		manager.mu.Unlock()
	}
	configFilename := runtimeFilename
	baseConfig := ""
	if manager.config.MCP != nil {
		result, err := manager.config.MCP.Reconcile(filename, mcp.BackendKobold)
		if err != nil {
			return err
		}
		if result.Enabled {
			configFilename = filepath.Join(".router-mcp", filename)
			baseConfig = runtimeFilename
		}
	}
	body, err := json.Marshal(map[string]string{
		"filename":       configFilename,
		"baseconfig":     baseConfig,
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
		manager.removeGenerated(generatedPath)
		return err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		manager.removeGenerated(generatedPath)
		return fmt.Errorf("admin reload failed with status %d: %s", response.StatusCode, reloadErrorDetail(responseBody))
	}

	var reload reloadResponse
	if err := json.Unmarshal(responseBody, &reload); err != nil {
		manager.removeGenerated(generatedPath)
		return err
	}
	if !reload.Success {
		manager.removeGenerated(generatedPath)
		if reload.Error != "" {
			return fmt.Errorf("admin reload failed: %s", reload.Error)
		}
		// KoboldCpp can report failure with no error field at all. Fall back to the raw
		// response rather than an unattributable "admin reload failed".
		return fmt.Errorf("admin reload failed: %s", reloadErrorDetail(responseBody))
	}

	manager.mu.Lock()
	exitDone := manager.exitDone
	manager.mu.Unlock()
	if err := manager.waitHealthy(ctx, 90*time.Second, exitDone); err != nil {
		manager.removeGenerated(generatedPath)
		return err
	}
	return nil
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

func (manager *Manager) waitHealthy(ctx context.Context, timeout time.Duration, exitDone <-chan error) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case err := <-exitDone:
			return unexpectedExitError("koboldcpp", err)
		default:
		}
		if manager.Healthy(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-exitDone:
			return unexpectedExitError("koboldcpp", err)
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("koboldcpp did not become healthy within %s", timeout)
}

func (manager *Manager) BackendExitError() error {
	manager.exitMu.RLock()
	defer manager.exitMu.RUnlock()
	if !manager.exitObserved {
		return nil
	}
	return unexpectedExitError("koboldcpp", manager.exitErr)
}

// backendExitedError marks a health-wait failure caused by the child process
// exiting, as opposed to it simply never becoming healthy in time. Only the
// former is worth retrying with a freshly allocated port: an exit signals
// the child failed outright (most commonly a lost race for a dynamic port),
// while a plain timeout means the process is alive and a retry would just
// waste the same 90 second wait again.
type backendExitedError struct {
	name string
	err  error
}

func (exitErr *backendExitedError) Error() string {
	if exitErr.err == nil {
		return fmt.Sprintf("%s exited during startup", exitErr.name)
	}
	return fmt.Sprintf("%s exited during startup: %s", exitErr.name, exitErr.err)
}

func (exitErr *backendExitedError) Unwrap() error {
	return exitErr.err
}

func unexpectedExitError(name string, err error) error {
	return &backendExitedError{name: name, err: err}
}

// reloadErrorDetail summarises an admin reload response body for an error message,
// bounded so a stray HTML error page cannot flood the log.
func reloadErrorDetail(body []byte) string {
	detail := strings.TrimSpace(string(body))
	if detail == "" {
		return "no detail reported"
	}
	const limit = 512
	if len(detail) > limit {
		detail = detail[:limit] + "…"
	}
	return detail
}

func (manager *Manager) BeginLoadCapture(maxOutputBytes int64) func() loadcapture.Capture {
	finish := manager.captureHub.Subscribe(maxOutputBytes)
	return func() loadcapture.Capture {
		capture := finish()
		capture.Secrets = []string{manager.adminPassword}
		return capture
	}
}

// WatchOutput delivers backend output to observe as it is produced, so a caller can
// decide readiness from what the process reports instead of polling an HTTP endpoint.
func (manager *Manager) WatchOutput(observe func(loadcapture.Stream, []byte)) func() {
	return manager.captureHub.Watch(observe)
}

func (manager *Manager) BeginLoadDiagnostic() func(bool) backenddiagnostic.Diagnostic {
	manager.capture.Begin()
	return manager.capture.End
}

func generateAdminPassword() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
