package vllm

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Backend struct {
	service   Service
	kind      RuntimeKind
	configDir string
	client    *http.Client
	mu        sync.Mutex
	filename  string
}

func NewBackend(service Service, kind RuntimeKind, configDir ...string) *Backend {
	directory := ""
	if len(configDir) > 0 {
		directory = configDir[0]
	}
	backend := &Backend{service: service, kind: kind, configDir: directory}
	backend.client = &http.Client{Transport: &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     false,
		DisableCompression:    true,
		MaxIdleConns:          16,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 5 * time.Minute,
		DialContext:           backend.dialContext,
	}}
	return backend
}

func (backend *Backend) URL() *url.URL {
	return &url.URL{Scheme: "http", Host: "vllm.local"}
}

func (backend *Backend) ReloadConfig(ctx context.Context, filename string) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	path := filename
	if backend.configDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(backend.configDir, path)
	}
	if backend.filename == path && backend.Healthy(ctx) {
		return nil
	}
	if backend.filename != "" {
		if err := backend.service.Unload(ctx, backend.kind); err != nil {
			return err
		}
	}
	_, err := backend.service.Load(ctx, RuntimeLoadRequest{Kind: backend.kind, ConfigPath: path})
	if err == nil {
		backend.filename = path
	}
	return err
}

func (backend *Backend) Restart(ctx context.Context) error {
	_, err := backend.service.Restart(ctx, backend.kind)
	return err
}

func (backend *Backend) Unload(ctx context.Context) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	err := backend.service.Unload(ctx, backend.kind)
	if err == nil {
		backend.filename = ""
	}
	return err
}

func (backend *Backend) Healthy(ctx context.Context) bool {
	status, err := backend.service.Runtime(ctx, backend.kind)
	return err == nil && status.Running && status.Healthy
}

func (backend *Backend) HTTPClient() *http.Client {
	return backend.client
}

func (backend *Backend) BackendExitError() error {
	status, err := backend.service.Runtime(context.Background(), backend.kind)
	if err != nil {
		return err
	}
	if status.Error != "" {
		return fmt.Errorf("vLLM %s runtime exited: %s", backend.kind, status.Error)
	}
	return nil
}

func (backend *Backend) dialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("vLLM private Unix socket is unsupported on Windows")
	}
	status, err := backend.service.Runtime(ctx, backend.kind)
	if err != nil {
		return nil, err
	}
	if !status.Running || strings.TrimSpace(status.SocketPath) == "" {
		return nil, fmt.Errorf("vLLM %s runtime is not running", backend.kind)
	}
	if err := validatePrivateSocket(status.SocketPath); err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, "unix", status.SocketPath)
}

func validatePrivateSocket(socketPath string) error {
	if !filepath.IsAbs(socketPath) {
		return fmt.Errorf("vLLM socket path must be absolute")
	}
	directoryInfo, err := os.Lstat(filepath.Dir(socketPath))
	if err != nil {
		return err
	}
	if !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("vLLM socket parent is not a real directory")
	}
	if runtime.GOOS != "windows" && directoryInfo.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("vLLM socket directory permissions must be 0700")
	}
	socketInfo, err := os.Lstat(socketPath)
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" && (socketInfo.Mode()&os.ModeSymlink != 0 || socketInfo.Mode()&os.ModeSocket == 0) {
		return fmt.Errorf("vLLM runtime endpoint is not a Unix socket")
	}
	if runtime.GOOS == "windows" && socketInfo.IsDir() {
		return fmt.Errorf("vLLM runtime endpoint is not a Unix socket")
	}
	return nil
}
