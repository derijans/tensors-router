package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/backenddiagnostic"
	"tensors-router/internal/backendendpoint"
	"tensors-router/internal/backendreadiness"
	"tensors-router/internal/catalog"
	"tensors-router/internal/loadcapture"
	"tensors-router/internal/mcp"
	"tensors-router/internal/portalloc"
	"tensors-router/internal/processcontrol"
)

type ProcessConfig struct {
	BackendURL     string
	BinaryPath     string
	ConfigDir      string
	DataDir        string
	ExtraArgs      []string
	HideWindow     bool
	Logging        bool
	MCP            *mcp.Reconciler
	VideoFFmpegDir string
}

type Manager struct {
	config          ProcessConfig
	endpoint        *backendendpoint.Endpoint
	readinessPath   string
	logName         string
	argumentBuilder argumentBuilder
	extraArgsFilter func(catalog.RuntimeConfig, []string) []string
	client          *http.Client
	mu              sync.Mutex
	exitMu          sync.RWMutex
	cmd             *exec.Cmd
	logFile         *os.File
	waitDone        chan error
	exitDone        <-chan error
	exitErr         error
	exitObserved    bool
	capture         *backenddiagnostic.Capture
	captureHub      *loadcapture.Hub
	currentFilename string
}

func NewLlamaManager(config ProcessConfig) (*Manager, error) {
	return newManager(config, "/v1/models", "llama-server.log", llamaArguments)
}

func NewLlamaEmbeddingsManager(config ProcessConfig) (*Manager, error) {
	manager, err := newManager(config, "/v1/models", "llama-server-embeddings.log", llamaEmbeddingArguments)
	if err != nil {
		return nil, err
	}
	manager.extraArgsFilter = embeddingExtraArgs
	return manager, nil
}

func NewSDCPPManager(config ProcessConfig) (*Manager, error) {
	return newManager(config, "/sdapi/v1/sd-models", "sd-server.log", sdcppArguments)
}

func NewWhisperCPPManager(config ProcessConfig) (*Manager, error) {
	if err := backendendpoint.RejectConflictingArgs(config.ExtraArgs, "--public", "--public-path", "--request-path", "--inference-path", "--convert", "--no-convert", "--tmp-dir"); err != nil {
		return nil, err
	}
	return newManager(config, "/health", "whisper-server.log", whisperCPPArguments)
}

func newManager(config ProcessConfig, readinessPath string, logName string, builder argumentBuilder) (*Manager, error) {
	endpoint, err := backendendpoint.NewEndpoint(config.BackendURL)
	if err != nil {
		return nil, err
	}
	if err := backendendpoint.RejectConflictingArgs(config.ExtraArgs, "--host", "--port", "--listen-ip", "--listen-port"); err != nil {
		return nil, err
	}
	return &Manager{
		config:          config,
		endpoint:        endpoint,
		readinessPath:   readinessPath,
		logName:         logName,
		argumentBuilder: builder,
		extraArgsFilter: identityArgs,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		capture:    backenddiagnostic.NewCapture(),
		captureHub: loadcapture.NewHub(),
	}, nil
}

func (manager *Manager) URL() *url.URL {
	return manager.endpoint.URL()
}

func (manager *Manager) LaunchArguments(filename string) ([]string, error) {
	if filename != filepath.Base(filename) {
		return nil, fmt.Errorf("config filename %q is invalid", filename)
	}
	metadata, err := catalog.LoadRuntimeConfig(filepath.Join(manager.config.ConfigDir, filename))
	if err != nil {
		return nil, err
	}
	host, port := manager.endpoint.HostPort()
	mcpServersPath := ""
	if manager.config.MCP != nil {
		result, err := manager.config.MCP.Reconcile(filename, mcp.BackendLlama)
		if err != nil {
			return nil, err
		}
		mcpServersPath = result.ServersPath
	}
	args, err := manager.argumentBuilder(metadata, launchTarget{
		modelID:        strings.TrimSuffix(filename, filepath.Ext(filename)),
		host:           host,
		port:           port,
		mcpServersPath: mcpServersPath,
		videoFFmpegDir: manager.config.VideoFFmpegDir,
	})
	if err != nil {
		return nil, err
	}
	args = append(args, manager.extraArgsFilter(metadata, manager.config.ExtraArgs)...)
	return args, nil
}

func (manager *Manager) ReloadConfig(ctx context.Context, filename string) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if manager.currentFilename == filename && manager.cmd != nil && manager.cmd.Process != nil && manager.healthy(ctx) {
		return nil
	}
	// Reserve (a no-op for a pinned or already-sticky-reserved endpoint)
	// and validate the config before touching the currently running
	// process, so a bad config leaves the existing backend untouched.
	if err := manager.endpoint.Reserve(portalloc.Default()); err != nil {
		return err
	}
	if _, err := manager.LaunchArguments(filename); err != nil {
		return err
	}
	if err := manager.stopLocked(ctx); err != nil {
		return err
	}
	return manager.startLocked(ctx, filename)
}

func (manager *Manager) Restart(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if manager.currentFilename == "" {
		return nil
	}
	filename := manager.currentFilename
	if err := manager.endpoint.Reserve(portalloc.Default()); err != nil {
		return err
	}
	if _, err := manager.LaunchArguments(filename); err != nil {
		return err
	}
	if err := manager.stopLocked(ctx); err != nil {
		return err
	}
	return manager.startLocked(ctx, filename)
}

func (manager *Manager) Unload(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.currentFilename = ""
	return manager.stopLocked(ctx)
}

// ReleaseEndpoint returns a dynamically allocated endpoint's port to the
// shared allocator. Call this only when the manager itself is being torn
// down for good (router shutdown) — Unload alone deliberately keeps a
// dynamic endpoint's port reserved so a later reload does not change the
// backend's address. It is a no-op for a pinned endpoint.
func (manager *Manager) ReleaseEndpoint() {
	manager.endpoint.Release(portalloc.Default())
}

func (manager *Manager) Healthy(ctx context.Context) bool {
	manager.mu.Lock()
	hasCurrent := manager.currentFilename != ""
	manager.mu.Unlock()
	if !hasCurrent {
		return true
	}
	return manager.healthy(ctx)
}

const maxPortAttempts = 3

func (manager *Manager) startLocked(ctx context.Context, filename string) error {
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
		args, err := manager.LaunchArguments(filename)
		if err != nil {
			return err
		}
		err = manager.spawnLocked(ctx, filename, args)
		if err == nil {
			return nil
		}
		var exitErr *backendExitedError
		if attempt >= maxPortAttempts || !manager.endpoint.Dynamic() || !errors.As(err, &exitErr) {
			return err
		}
		// The child exited during startup on a dynamically allocated port,
		// the signature of a lost race for that port. Free it and retry
		// with a freshly reserved one.
		manager.endpoint.Release(portalloc.Default())
	}
}

func (manager *Manager) spawnLocked(ctx context.Context, filename string, args []string) error {
	var logFile *os.File
	stdout := io.MultiWriter(manager.capture, manager.captureHub.Stdout())
	stderr := io.MultiWriter(manager.capture, manager.captureHub.Stderr())
	if manager.config.Logging {
		logPath := filepath.Join(manager.config.DataDir, manager.logName)
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		stdout = io.MultiWriter(stdout, logFile)
		stderr = io.MultiWriter(stderr, logFile)
	}

	cmd := exec.Command(manager.config.BinaryPath, args...)
	cmd.Env = nativeProcessEnv(manager.config.BinaryPath, os.Environ())
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
	manager.currentFilename = filename
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
		manager.currentFilename = ""
		return err
	}

	return nil
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

func (manager *Manager) healthy(ctx context.Context) bool {
	target := manager.URL()
	target.Path = joinPath(target.Path, manager.readinessPath)
	target.RawQuery = ""

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
			return manager.exitError(err)
		default:
		}
		if manager.healthy(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-exitDone:
			return manager.exitError(err)
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("native server did not become healthy within %s", timeout)
}

func (manager *Manager) BackendExitError() error {
	manager.exitMu.RLock()
	defer manager.exitMu.RUnlock()
	if !manager.exitObserved {
		return nil
	}
	return manager.exitError(manager.exitErr)
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
	// detail is the decisive line from the backend's own output, so the failure names
	// its cause rather than only an exit status.
	detail string
}

func (exitErr *backendExitedError) Error() string {
	message := fmt.Sprintf("%s exited during startup", exitErr.name)
	if exitErr.err != nil {
		message = fmt.Sprintf("%s: %s", message, exitErr.err)
	}
	if exitErr.detail != "" {
		message = fmt.Sprintf("%s: %s", message, exitErr.detail)
	}
	return message
}

func (exitErr *backendExitedError) Unwrap() error {
	return exitErr.err
}

// exitError builds the startup failure for this manager, enriched with the decisive line
// from the captured output when the backend explained itself before dying.
func (manager *Manager) exitError(err error) error {
	detail := backendreadiness.DecisiveFailureLine(manager.capture.Snapshot().Output)
	return &backendExitedError{name: "native server", err: err, detail: detail}
}

func (manager *Manager) BeginLoadCapture(maxOutputBytes int64) func() loadcapture.Capture {
	return manager.captureHub.Subscribe(maxOutputBytes)
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

func nativeProcessEnv(binaryPath string, baseEnv []string) []string {
	binaryDir := filepath.Dir(strings.TrimSpace(binaryPath))
	if binaryDir == "" {
		return baseEnv
	}
	if absoluteDir, err := filepath.Abs(binaryDir); err == nil {
		binaryDir = absoluteDir
	}
	return prependEnvPath(baseEnv, nativeLibraryPathEnvName(), binaryDir)
}

func nativeLibraryPathEnvName() string {
	switch runtime.GOOS {
	case "windows":
		return "PATH"
	case "darwin":
		return "DYLD_LIBRARY_PATH"
	default:
		return "LD_LIBRARY_PATH"
	}
}

func prependEnvPath(baseEnv []string, name string, path string) []string {
	if strings.TrimSpace(path) == "" {
		return baseEnv
	}
	prefix := name + "="
	for index, value := range baseEnv {
		key, current, ok := strings.Cut(value, "=")
		if ok && envNameMatches(key, name) {
			updated := path
			if current != "" {
				updated += string(os.PathListSeparator) + current
			}
			env := append([]string{}, baseEnv...)
			env[index] = key + "=" + updated
			return env
		}
	}
	env := append([]string{}, baseEnv...)
	env = append(env, prefix+path)
	return env
}

func envNameMatches(key string, name string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(key, name)
	}
	return key == name
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

func joinPath(base string, requestPath string) string {
	if base == "" || base == "/" {
		return requestPath
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(requestPath, "/")
}
