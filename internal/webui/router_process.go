package webui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/siteapi"
)

type RouterProcess struct {
	mu       sync.Mutex
	config   RouterConfig
	url      string
	managed  bool
	cmd      *exec.Cmd
	waitDone chan struct{}
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
	status := siteapi.RouterProcessStatus{
		Managed: process.managed,
		URL:     process.url,
		Error:   process.lastErr,
	}
	if process.cmd != nil && process.cmd.Process != nil {
		status.PID = process.cmd.Process.Pid
	}
	process.mu.Unlock()
	status.Running = process.Healthy(ctx)
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
	args := []string{"serve", "--config", process.config.ConfigPath}
	args = append(args, process.config.Args...)
	cmd := exec.CommandContext(context.Background(), binaryPath, args...)
	cmd.Dir = filepath.Dir(binaryPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		process.lastErr = err.Error()
		process.mu.Unlock()
		return err
	}
	waitDone := make(chan struct{})
	process.cmd = cmd
	process.waitDone = waitDone
	process.lastErr = ""
	go process.waitForExit(cmd, waitDone)
	process.mu.Unlock()
	return process.waitHealthy(ctx, 30*time.Second)
}

func (process *RouterProcess) Kill(ctx context.Context) error {
	process.mu.Lock()
	if !process.managed {
		process.mu.Unlock()
		return fmt.Errorf("router process is external")
	}
	cmd := process.cmd
	waitDone := process.waitDone
	process.cmd = nil
	process.waitDone = nil
	process.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return stopRouterProcess(ctx, cmd, waitDone)
}

func (process *RouterProcess) Restart(ctx context.Context) error {
	if err := process.Kill(ctx); err != nil {
		return err
	}
	return process.Launch(ctx)
}

func (process *RouterProcess) Shutdown(ctx context.Context) error {
	if !process.config.ShutdownWithWebUI {
		return nil
	}
	return process.Kill(ctx)
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

func (process *RouterProcess) waitForExit(cmd *exec.Cmd, waitDone chan<- struct{}) {
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
	close(waitDone)
}

func stopRouterProcess(ctx context.Context, cmd *exec.Cmd, waitDone <-chan struct{}) error {
	if runtime.GOOS == "windows" {
		_ = cmd.Process.Kill()
		if waitDone != nil {
			<-waitDone
		}
		return nil
	}
	_ = cmd.Process.Signal(os.Interrupt)
	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return ctx.Err()
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		return nil
	case <-waitDone:
		return nil
	}
}
