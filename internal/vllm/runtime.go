package vllm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type runtimeProcess struct {
	status RuntimeStatus
	child  RuntimeChild
	logs   *boundedLog
	config RuntimeLoadRequest
}

func (manager *Manager) Load(ctx context.Context, request RuntimeLoadRequest) (RuntimeStatus, error) {
	if err := validateRuntimeKind(request.Kind); err != nil {
		return RuntimeStatus{}, err
	}
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return RuntimeStatus{}, fmt.Errorf("vLLM manager is closed")
	}
	if manager.active.Path == "" {
		manager.mu.Unlock()
		return RuntimeStatus{}, fmt.Errorf("backend_not_initialized")
	}
	status, reusable, staleSocketPath, err := prepareRuntimeLoad(manager.runtimes, request)
	if err != nil {
		manager.mu.Unlock()
		return RuntimeStatus{}, err
	}
	if reusable {
		manager.mu.Unlock()
		return status, nil
	}
	active := manager.active
	manager.mu.Unlock()
	if staleSocketPath != "" {
		_ = os.Remove(staleSocketPath)
	}

	configuration, err := LoadModelConfig(request.ConfigPath)
	if err != nil {
		return RuntimeStatus{}, err
	}
	configuration.ServedNames = ensureRouterServedName(configuration.ServedNames, strings.TrimSuffix(filepath.Base(request.ConfigPath), filepath.Ext(request.ConfigPath)))
	if err := manager.validateModelSecurityPolicy(configuration); err != nil {
		return RuntimeStatus{}, err
	}
	if err := VerifySnapshot(configuration.Snapshot); err != nil {
		return RuntimeStatus{}, err
	}
	for _, adapter := range configuration.StaticAdapters {
		if err := VerifySnapshot(SnapshotIdentity{Path: adapter.Path, TreeDigest: adapter.TreeDigest}); err != nil {
			return RuntimeStatus{}, fmt.Errorf("verify static adapter %q: %w", adapter.Name, err)
		}
	}
	socketDirectory := filepath.Join(manager.dataDir, "sockets", string(request.Kind))
	if err := ensurePrivateDirectory(socketDirectory); err != nil {
		return RuntimeStatus{}, err
	}
	socketPath := filepath.Join(socketDirectory, "vllm.sock")
	if runtime.GOOS == "windows" {
		return RuntimeStatus{}, fmt.Errorf("vLLM runtime requires Unix-domain socket support")
	}
	_ = os.Remove(socketPath)
	logs := newBoundedLog(maximumRuntimeLogBytes)
	executable, arguments, environment, err := runtimeLaunchCommand(active, configuration, socketPath, manager.options.AllowDynamicLoRA)
	if err != nil {
		return RuntimeStatus{}, err
	}
	child, err := manager.options.RuntimeLauncher.Start(context.Background(), executable, arguments, environment, active.Path, logs)
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("start vLLM %s runtime: %w", request.Kind, err)
	}
	process := &runtimeProcess{
		status: RuntimeStatus{Kind: request.Kind, Running: true, SocketPath: socketPath, ModelID: primaryServedName(configuration), Version: active.VLLMVersion},
		child:  child,
		logs:   logs,
		config: request,
	}
	manager.mu.Lock()
	if manager.closed || manager.runtimes[request.Kind] != nil {
		manager.mu.Unlock()
		stopContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = child.Stop(stopContext)
		return RuntimeStatus{}, fmt.Errorf("vLLM %s runtime load conflict", request.Kind)
	}
	manager.runtimes[request.Kind] = process
	manager.mu.Unlock()
	go manager.watchRuntime(request.Kind, process)
	readyStatus, err := manager.waitForRuntimeSocket(ctx, request.Kind, process)
	if err != nil {
		stopContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = process.child.Stop(stopContext)
		cancel()
		manager.mu.Lock()
		if manager.runtimes[request.Kind] == process {
			delete(manager.runtimes, request.Kind)
		}
		manager.mu.Unlock()
		return RuntimeStatus{}, err
	}
	return readyStatus, nil
}

func prepareRuntimeLoad(runtimes map[RuntimeKind]*runtimeProcess, request RuntimeLoadRequest) (RuntimeStatus, bool, string, error) {
	loaded := runtimes[request.Kind]
	if loaded == nil {
		return RuntimeStatus{}, false, "", nil
	}
	if loaded.config.ConfigPath != request.ConfigPath {
		return RuntimeStatus{}, false, "", fmt.Errorf("vLLM %s runtime already loaded", request.Kind)
	}
	if loaded.status.Running {
		return loaded.status, true, "", nil
	}
	delete(runtimes, request.Kind)
	return RuntimeStatus{}, false, loaded.status.SocketPath, nil
}

func ensureRouterServedName(servedNames []string, routerModelID string) []string {
	routerModelID = strings.TrimSpace(routerModelID)
	if routerModelID == "" {
		return servedNames
	}
	for _, servedName := range servedNames {
		if servedName == routerModelID {
			return servedNames
		}
	}
	return append([]string{routerModelID}, servedNames...)
}

func runtimeLaunchCommand(active activeEnvironment, configuration VLLMModelConfig, socketPath string, dynamicLoRA bool) (string, []string, []string, error) {
	if active.InstallMethod != "oci" {
		arguments, err := BuildServeArguments(configuration, socketPath, dynamicLoRA)
		if err != nil {
			return "", nil, nil, err
		}
		environment := isolatedRuntimeEnvironment(active.Path, configuration.Snapshot.Path)
		if dynamicLoRA {
			environment = append(environment, "VLLM_ALLOW_RUNTIME_LORA_UPDATING=True")
		}
		return environmentPythonPath(active.Path), arguments, environment, nil
	}
	if !validOCIImage(active.OCIImage) || active.ContainerEngine != "docker" && active.ContainerEngine != "podman" {
		return "", nil, nil, fmt.Errorf("active OCI runtime metadata is invalid")
	}
	enginePath, engineName, err := findContainerEngine(active.ContainerEngine)
	if err != nil {
		return "", nil, nil, err
	}
	if engineName != active.ContainerEngine {
		return "", nil, nil, fmt.Errorf("active OCI container engine is unavailable")
	}
	containerConfiguration, mounts, err := containerModelConfiguration(configuration, socketPath)
	if err != nil {
		return "", nil, nil, err
	}
	containerSocket := "/router-socket/vllm.sock"
	command, err := BuildServeArguments(containerConfiguration, containerSocket, dynamicLoRA)
	if err != nil {
		return "", nil, nil, err
	}
	containerEnvironment := []string{}
	if dynamicLoRA {
		containerEnvironment = append(containerEnvironment, "VLLM_ALLOW_RUNTIME_LORA_UPDATING=True")
	}
	profile := Profile{OCIImage: active.OCIImage, Devices: append([]string{}, active.Devices...)}
	arguments := ociCommandArguments(engineName, profile, mounts, containerEnvironment, command, len(configuration.ExternalToolServers) > 0)
	return enginePath, arguments, containerEngineEnvironment(active.Path), nil
}

func containerModelConfiguration(configuration VLLMModelConfig, socketPath string) (VLLMModelConfig, []ociMount, error) {
	socketDirectory := filepath.Dir(socketPath)
	if err := validateOCIMountPath(socketDirectory); err != nil {
		return VLLMModelConfig{}, nil, err
	}
	if err := validateOCIMountPath(configuration.Snapshot.Path); err != nil {
		return VLLMModelConfig{}, nil, err
	}
	mounts := []ociMount{
		{Source: socketDirectory, Destination: "/router-socket"},
		{Source: configuration.Snapshot.Path, Destination: "/models/model", ReadOnly: true},
	}
	configuration.Snapshot.Path = "/models/model"
	for index := range configuration.StaticAdapters {
		if err := validateOCIMountPath(configuration.StaticAdapters[index].Path); err != nil {
			return VLLMModelConfig{}, nil, err
		}
		destination := fmt.Sprintf("/models/adapters/%d", index)
		mounts = append(mounts, ociMount{Source: configuration.StaticAdapters[index].Path, Destination: destination, ReadOnly: true})
		configuration.StaticAdapters[index].Path = destination
	}
	return configuration, mounts, nil
}

func validateOCIMountPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsAny(path, ",\x00\r\n") {
		return fmt.Errorf("OCI mount path is invalid")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("OCI mount path must be a real directory")
	}
	return nil
}

func (manager *Manager) validateModelSecurityPolicy(configuration VLLMModelConfig) error {
	if configuration.TrustRemoteCode && !manager.options.AllowTrustRemoteCode {
		return fmt.Errorf("trust_remote_code requires explicit administrator configuration")
	}
	if len(configuration.ExternalToolServers) > 0 && !manager.options.AllowExternalTools {
		return fmt.Errorf("external tool servers require explicit administrator configuration")
	}
	return nil
}

func (manager *Manager) waitForRuntimeSocket(ctx context.Context, kind RuntimeKind, process *runtimeProcess) (RuntimeStatus, error) {
	deadline := time.NewTimer(2 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if validatePrivateSocket(process.status.SocketPath) == nil {
			if err := os.Chmod(process.status.SocketPath, 0o600); err != nil {
				return RuntimeStatus{}, fmt.Errorf("secure vLLM socket: %w", err)
			}
			probeContext, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := probeRuntimeHealth(probeContext, process.status.SocketPath)
			cancel()
			if err == nil {
				manager.mu.Lock()
				if manager.runtimes[kind] == process {
					process.status.Healthy = true
				}
				status := process.status
				manager.mu.Unlock()
				return status, nil
			}
		}
		manager.mu.Lock()
		running := manager.runtimes[kind] == process && process.status.Running
		exitError := process.status.Error
		manager.mu.Unlock()
		if !running {
			return RuntimeStatus{}, fmt.Errorf("vLLM %s runtime exited before health check succeeded: %s", kind, exitError)
		}
		select {
		case <-ctx.Done():
			return RuntimeStatus{}, ctx.Err()
		case <-deadline.C:
			return RuntimeStatus{}, fmt.Errorf("vLLM %s runtime did not pass private /health check", kind)
		case <-ticker.C:
		}
	}
}

func (manager *Manager) Restart(ctx context.Context, kind RuntimeKind) (RuntimeStatus, error) {
	manager.mu.Lock()
	process := manager.runtimes[kind]
	manager.mu.Unlock()
	if process == nil {
		return RuntimeStatus{}, fmt.Errorf("vLLM %s runtime is not loaded", kind)
	}
	request := process.config
	if err := manager.Unload(ctx, kind); err != nil {
		return RuntimeStatus{}, err
	}
	return manager.Load(ctx, request)
}

func (manager *Manager) Unload(ctx context.Context, kind RuntimeKind) error {
	if err := validateRuntimeKind(kind); err != nil {
		return err
	}
	manager.mu.Lock()
	process := manager.runtimes[kind]
	if process == nil {
		manager.mu.Unlock()
		return nil
	}
	delete(manager.runtimes, kind)
	manager.mu.Unlock()
	stopContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := process.child.Stop(stopContext)
	_ = os.Remove(process.status.SocketPath)
	return err
}

func (manager *Manager) Runtime(_ context.Context, kind RuntimeKind) (RuntimeStatus, error) {
	if err := validateRuntimeKind(kind); err != nil {
		return RuntimeStatus{}, err
	}
	manager.mu.Lock()
	process := manager.runtimes[kind]
	if process == nil {
		manager.mu.Unlock()
		return RuntimeStatus{Kind: kind}, nil
	}
	status := process.status
	status.Logs = process.logs.String()
	manager.mu.Unlock()
	if status.Running {
		probeContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		healthError := probeRuntimeHealth(probeContext, status.SocketPath)
		cancel()
		status.Healthy = healthError == nil
		manager.mu.Lock()
		if manager.runtimes[kind] == process {
			process.status.Healthy = status.Healthy
		}
		manager.mu.Unlock()
	}
	return status, nil
}

func probeRuntimeHealth(ctx context.Context, socketPath string) error {
	if err := validatePrivateSocket(socketPath); err != nil {
		return err
	}
	client := unixHTTPClient(socketPath, 2*time.Second)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://vllm.local/health", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if err := response.Body.Close(); err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("vLLM /health returned HTTP %d", response.StatusCode)
	}
	return nil
}

func unixHTTPClient(socketPath string, timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		DialContext: func(dialContext context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: timeout}).DialContext(dialContext, "unix", socketPath)
		},
	}
	return &http.Client{Transport: transport, Timeout: timeout}
}

func (manager *Manager) Close() error {
	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return nil
	}
	manager.closed = true
	if manager.worker != nil {
		manager.worker.cancel()
		manager.worker = nil
	}
	runtimes := make(map[RuntimeKind]*runtimeProcess, len(manager.runtimes))
	for kind, process := range manager.runtimes {
		runtimes[kind] = process
	}
	manager.runtimes = map[RuntimeKind]*runtimeProcess{}
	closeWait := manager.closeWait
	if closeWait <= 0 {
		closeWait = 10 * time.Second
	}
	manager.mu.Unlock()
	closeContext, closeCancel := context.WithTimeout(context.Background(), closeWait)
	defer closeCancel()
	var closeError error
	workersDone := make(chan struct{})
	go func() {
		manager.workers.Wait()
		close(workersDone)
	}()
	select {
	case <-workersDone:
	case <-closeContext.Done():
		closeError = errors.Join(closeError, fmt.Errorf("wait for vLLM initialization cleanup: %w", closeContext.Err()))
	}
	stopResults := make(chan error, len(runtimes))
	for _, process := range runtimes {
		go func(process *runtimeProcess) {
			stopResults <- process.child.Stop(closeContext)
		}(process)
		_ = os.Remove(process.status.SocketPath)
	}
	for range runtimes {
		select {
		case err := <-stopResults:
			closeError = errors.Join(closeError, err)
		case <-closeContext.Done():
			closeError = errors.Join(closeError, fmt.Errorf("stop vLLM runtimes: %w", closeContext.Err()))
			return closeError
		}
	}
	return closeError
}

func (manager *Manager) watchRuntime(kind RuntimeKind, process *runtimeProcess) {
	err := process.child.Wait()
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.runtimes[kind] != process {
		return
	}
	process.status.Running = false
	process.status.Healthy = false
	process.status.Error = errorText(sanitizeError(err))
	process.status.Logs = process.logs.String()
}

func validateRuntimeKind(kind RuntimeKind) error {
	switch kind {
	case RuntimeGeneration, RuntimePooling, RuntimeSpeech:
		return nil
	default:
		return fmt.Errorf("unsupported vLLM runtime kind %q", kind)
	}
}

func primaryServedName(configuration VLLMModelConfig) string {
	if len(configuration.ServedNames) > 0 {
		return configuration.ServedNames[0]
	}
	return filepath.Base(configuration.Snapshot.Path)
}

func safeServedName(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.HasPrefix(value, "-") || strings.ContainsAny(value, "=\x00\r\n\t ") {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			continue
		}
		switch character {
		case '-', '.', '_', '/', ':', '@', '+':
			continue
		default:
			return false
		}
	}
	return true
}

func safeAdapterName(value string) bool {
	return safeServedName(value) && !strings.Contains(value, "/")
}
