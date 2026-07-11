package webui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/processcontrol"
	"tensors-router/internal/siteapi"
)

const (
	managedRouterDrainGrace  = 16 * time.Minute
	managedRouterKillTimeout = 5 * time.Second
)

type RouterProcess struct {
	mu       sync.Mutex
	config   RouterConfig
	url      string
	managed  bool
	cmd      *exec.Cmd
	waitDone chan error
	lastErr  string
	client   *http.Client
}

func NewRouterProcess(config RouterConfig, executableDir string) *RouterProcess {
	urlValue := strings.TrimSpace(config.URL)
	managed := false
	if urlValue == "" {
		urlValue = "http://127.0.0.1:8080"
		managed = true
		if strings.TrimSpace(config.BinaryPath) == "" {
			config.BinaryPath = filepath.Join(executableDir, routerExecutableName())
		}
	}
	return &RouterProcess{
		config:  config,
		url:     urlValue,
		managed: managed,
		client:  &http.Client{Timeout: 2 * time.Second},
	}
}

func (process *RouterProcess) URL() string {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.url
}

func (process *RouterProcess) Managed() bool {
	process.mu.Lock()
	defer process.mu.Unlock()
	return process.managed
}

func (process *RouterProcess) Status(ctx context.Context) siteapi.RouterProcessStatus {
	process.mu.Lock()
	managed := process.managed
	routerURL := process.url
	token := strings.TrimSpace(process.config.Token)
	hasProcess := process.cmd != nil && process.cmd.Process != nil
	status := siteapi.RouterProcessStatus{
		Managed: managed,
		URL:     routerURL,
		Error:   process.lastErr,
	}
	if hasProcess {
		status.PID = process.cmd.Process.Pid
	}
	process.mu.Unlock()
	status.Running = process.Healthy(ctx)
	status.CanShutdown = (managed && hasProcess) || (!managed && status.Running && token != "")
	status.CanForceKill = managed && hasProcess
	return status
}

func (process *RouterProcess) Healthy(ctx context.Context) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(process.URL(), "/")+"/router/v1/models", nil)
	if err != nil {
		return false
	}
	response, err := process.client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode >= 200 && response.StatusCode < 500
}

func (process *RouterProcess) EnsureStarted(ctx context.Context) error {
	if !process.Managed() || !process.config.StartWhenMissing || process.Healthy(ctx) {
		return nil
	}
	return process.Launch(ctx)
}

func (process *RouterProcess) Launch(ctx context.Context) error {
	process.mu.Lock()
	if !process.managed {
		process.mu.Unlock()
		return fmt.Errorf("router process is external")
	}
	if process.cmd != nil && process.cmd.Process != nil {
		process.mu.Unlock()
		return nil
	}
	binaryPath := strings.TrimSpace(process.config.BinaryPath)
	if binaryPath == "" {
		process.mu.Unlock()
		return fmt.Errorf("router binary path is required")
	}
	if _, err := os.Stat(binaryPath); err != nil {
		process.lastErr = err.Error()
		process.mu.Unlock()
		return err
	}
	args := routerLaunchArguments(process.config)
	cmd := exec.CommandContext(context.Background(), binaryPath, args...)
	cmd.Dir = filepath.Dir(binaryPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := processcontrol.Start(cmd, processcontrol.Options{ParentDeathGracePeriod: managedRouterDrainGrace}); err != nil {
		process.lastErr = err.Error()
		process.mu.Unlock()
		return err
	}
	waitDone := make(chan error, 1)
	process.cmd = cmd
	process.waitDone = waitDone
	process.lastErr = ""
	go process.waitForExit(cmd, waitDone)
	process.mu.Unlock()
	return process.waitHealthy(ctx, 30*time.Second)
}

func routerLaunchArguments(config RouterConfig) []string {
	args := []string{"serve", "--config", config.ConfigPath}
	args = append(args, config.Args...)
	if profile := strings.TrimSpace(config.SecurityProfile); profile != "" {
		args = append(args, "--security-profile", profile)
	}
	return args
}

func (process *RouterProcess) Kill(ctx context.Context) error {
	return process.GracefulShutdown(ctx)
}

func (process *RouterProcess) GracefulShutdown(ctx context.Context) error {
	if !process.Managed() {
		return process.shutdownExternal(ctx)
	}
	cmd, waitDone, err := process.takeManagedCommand()
	if err != nil {
		return err
	}
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return stopRouterProcess(ctx, cmd, waitDone)
}

func (process *RouterProcess) ForceKill() error {
	cmd, waitDone, err := process.takeManagedCommand()
	if err != nil {
		return err
	}
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return forceRouterProcess(cmd, waitDone)
}

func (process *RouterProcess) takeManagedCommand() (*exec.Cmd, <-chan error, error) {
	process.mu.Lock()
	defer process.mu.Unlock()
	if !process.managed {
		return nil, nil, fmt.Errorf("router process is external")
	}
	cmd := process.cmd
	waitDone := process.waitDone
	process.cmd = nil
	process.waitDone = nil
	return cmd, waitDone, nil
}

func (process *RouterProcess) Restart(ctx context.Context) error {
	if err := process.Kill(ctx); err != nil {
		return err
	}
	return process.Launch(ctx)
}

func (process *RouterProcess) Shutdown(ctx context.Context) error {
	if !process.config.ShutdownWithWebUI || !process.Managed() {
		return nil
	}
	return process.GracefulShutdown(ctx)
}

func (process *RouterProcess) shutdownExternal(ctx context.Context) error {
	token := strings.TrimSpace(process.config.Token)
	if token == "" {
		return fmt.Errorf("router token is required for external shutdown")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(process.URL(), "/")+"/router/v1/shutdown", nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := process.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("router shutdown failed with status %d", response.StatusCode)
	}
	return nil
}

func (process *RouterProcess) waitHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if process.Healthy(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("router did not become healthy within %s", timeout)
}

func (process *RouterProcess) waitForExit(cmd *exec.Cmd, waitDone chan<- error) {
	err := cmd.Wait()
	process.mu.Lock()
	if process.cmd == cmd {
		process.cmd = nil
		process.waitDone = nil
	}
	if err != nil {
		process.lastErr = err.Error()
	}
	process.mu.Unlock()
	waitDone <- err
	close(waitDone)
}

func stopRouterProcess(ctx context.Context, cmd *exec.Cmd, waitDone <-chan error) error {
	return processcontrol.Stop(ctx, cmd, waitDone, managedRouterDrainGrace, managedRouterKillTimeout)
}

func forceRouterProcess(cmd *exec.Cmd, waitDone <-chan error) error {
	return processcontrol.ForceStop(cmd, waitDone, managedRouterKillTimeout)
}
