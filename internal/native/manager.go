package native

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"tensors-router/internal/backendendpoint"
	"tensors-router/internal/catalog"
	"tensors-router/internal/processcontrol"
)

type ProcessConfig struct {
	BackendURL string
	BinaryPath string
	ConfigDir  string
	DataDir    string
	ExtraArgs  []string
	HideWindow bool
	Logging    bool
}

type Manager struct {
	config          ProcessConfig
	backendURL      *url.URL
	defaultPort     string
	readinessPath   string
	logName         string
	argumentBuilder func(catalog.RuntimeConfig, string, string, string) ([]string, error)
	client          *http.Client
	mu              sync.Mutex
	cmd             *exec.Cmd
	logFile         *os.File
	waitDone        chan error
	currentFilename string
}

func NewLlamaManager(config ProcessConfig) (*Manager, error) {
	return newManager(config, "5002", "/v1/models", "llama-server.log", llamaArguments)
}

func NewSDCPPManager(config ProcessConfig) (*Manager, error) {
	return newManager(config, "7860", "/sdapi/v1/sd-models", "sd-server.log", sdcppArguments)
}

func newManager(config ProcessConfig, defaultPort string, readinessPath string, logName string, argumentBuilder func(catalog.RuntimeConfig, string, string, string) ([]string, error)) (*Manager, error) {
	backendURL, err := backendendpoint.ParseLoopback(config.BackendURL)
	if err != nil {
		return nil, err
	}
	if err := backendendpoint.RejectConflictingArgs(config.ExtraArgs, "--host", "--port", "--listen-ip", "--listen-port"); err != nil {
		return nil, err
	}
	return &Manager{
		config:          config,
		backendURL:      backendURL,
		defaultPort:     defaultPort,
		readinessPath:   readinessPath,
		logName:         logName,
		argumentBuilder: argumentBuilder,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (manager *Manager) URL() *url.URL {
	copyValue := *manager.backendURL
	return &copyValue
}

func (manager *Manager) LaunchArguments(filename string) ([]string, error) {
	filename = strings.TrimSpace(filename)
	if filename != filepath.Base(filename) {
		return nil, fmt.Errorf("config filename %q is invalid", filename)
	}
	metadata, err := catalog.LoadRuntimeConfig(filepath.Join(manager.config.ConfigDir, filename))
	if err != nil {
		return nil, err
	}
	host, port := manager.hostPort()
	args, err := manager.argumentBuilder(metadata, strings.TrimSuffix(filename, filepath.Ext(filename)), host, port)
	if err != nil {
		return nil, err
	}
	args = append(args, manager.config.ExtraArgs...)
	return args, nil
}

func (manager *Manager) ReloadConfig(ctx context.Context, filename string) error {
	args, err := manager.LaunchArguments(filename)
	if err != nil {
		return err
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	if manager.currentFilename == filename && manager.cmd != nil && manager.cmd.Process != nil && manager.healthy(ctx) {
		return nil
	}
	if err := manager.stopLocked(ctx); err != nil {
		return err
	}
	return manager.startLocked(ctx, filename, args)
}

func (manager *Manager) Restart(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	if manager.currentFilename == "" {
		return nil
	}
	filename := manager.currentFilename
	args, err := manager.LaunchArguments(filename)
	if err != nil {
		return err
	}
	if err := manager.stopLocked(ctx); err != nil {
		return err
	}
	return manager.startLocked(ctx, filename, args)
}

func (manager *Manager) Unload(ctx context.Context) error {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.currentFilename = ""
	return manager.stopLocked(ctx)
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

func (manager *Manager) startLocked(ctx context.Context, filename string, args []string) error {
	if err := os.MkdirAll(manager.config.DataDir, 0o755); err != nil {
		return err
	}

	var logFile *os.File
	processOutput := io.Writer(io.Discard)
	if manager.config.Logging {
		logPath := filepath.Join(manager.config.DataDir, manager.logName)
		var err error
		logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		processOutput = logFile
	}

	cmd := exec.Command(manager.config.BinaryPath, args...)
	cmd.Env = nativeProcessEnv(manager.config.BinaryPath, os.Environ())
	cmd.Stdout = processOutput
	cmd.Stderr = processOutput

	if err := processcontrol.Start(cmd, processcontrol.Options{HideWindow: manager.config.HideWindow, ParentDeathGracePeriod: 10 * time.Second}); err != nil {
		_ = closeLogFile(logFile)
		return err
	}

	manager.cmd = cmd
	manager.logFile = logFile
	manager.currentFilename = filename
	waitDone := make(chan error, 1)
	manager.waitDone = waitDone

	go func() {
		waitDone <- cmd.Wait()
		_ = closeLogFile(logFile)
	}()

	if err := manager.waitHealthy(ctx, 90*time.Second); err != nil {
		_ = processcontrol.Kill(cmd)
		manager.cmd = nil
		manager.logFile = nil
		manager.waitDone = nil
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

func (manager *Manager) waitHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if manager.healthy(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("native server did not become healthy within %s", timeout)
}

func (manager *Manager) hostPort() (string, string) {
	host := manager.backendURL.Hostname()
	port := manager.backendURL.Port()
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = manager.defaultPort
	}
	if parsed := net.ParseIP(host); parsed != nil && parsed.IsUnspecified() {
		host = "127.0.0.1"
	}
	return host, port
}

func llamaArguments(metadata catalog.RuntimeConfig, modelID string, host string, port string) ([]string, error) {
	modelPath := metadata.TextModelPath()
	if modelPath == "" {
		return nil, fmt.Errorf("llama config has no text, embedding, or multimodal model path")
	}
	args := []string{
		"--host", host,
		"--port", port,
		"--model", modelPath,
		"--alias", modelID,
	}
	appendIntArg(&args, "--ctx-size", metadata.ContextSize)
	appendIntArg(&args, "--threads", metadata.Threads)
	appendIntArg(&args, "--threads-batch", metadata.BLASThreads)
	appendIntArg(&args, "--batch-size", metadata.BatchSize)
	appendIntArg(&args, "--ubatch-size", metadata.UBatchSize)
	appendIntArg(&args, "--n-gpu-layers", metadata.GPULayers)
	appendStringArg(&args, "--split-mode", metadata.SplitMode)
	appendStringArg(&args, "--tensor-split", metadata.TensorSplitValue())
	appendIntArg(&args, "--main-gpu", nonNegative(metadata.MainGPU))
	appendIntArg(&args, "--parallel", metadata.Parallel)
	appendOptionalBoolArg(&args, "--cont-batching", "--no-cont-batching", metadata.ContBatching)
	appendIntArg(&args, "--cache-ram", metadata.CacheRAM)
	appendIntArg(&args, "--ctx-checkpoints", metadata.CtxCheckpoints)
	appendOptionalBoolArg(&args, "--kv-unified", "--no-kv-unified", metadata.KVUnified)
	appendOptionalBoolArg(&args, "--cache-idle-slots", "--no-cache-idle-slots", metadata.CacheIdleSlots)
	if metadata.SWAFull {
		args = append(args, "--swa-full")
	}
	appendStringArg(&args, "--spec-type", metadata.SpecType)
	appendStringArg(&args, "--spec-draft-type-k", metadata.SpecDraftTypeK)
	appendStringArg(&args, "--spec-draft-type-v", metadata.SpecDraftTypeV)
	if !metadata.UseMMap {
		args = append(args, "--no-mmap")
	}
	if metadata.UseMLock {
		args = append(args, "--mlock")
	}
	if strings.TrimSpace(metadata.EmbeddingsModel) != "" {
		args = append(args, "--embeddings")
	}
	appendStringArg(&args, "--cache-type-k", firstNonEmpty(metadata.CacheTypeK, metadata.QuantKV))
	appendStringArg(&args, "--cache-type-v", firstNonEmpty(metadata.CacheTypeV, metadata.QuantKV))
	if mmproj := metadata.MMProjPath(); mmproj != "" {
		args = append(args, "--mmproj", mmproj)
	}
	if metadata.MMProjCPU {
		args = append(args, "--no-mmproj-offload")
	}
	appendIntArg(&args, "--image-min-tokens", positive(metadata.VisionMinTokens))
	appendIntArg(&args, "--image-max-tokens", positive(metadata.VisionMaxTokens))
	appendStringArg(&args, "--model-vocoder", metadata.Code2WAVModel)
	appendStringArg(&args, "--api-key-file", metadata.APIKeyFile)
	appendStringArg(&args, "--log-prompts-dir", metadata.LogPromptsDir)
	if metadata.Agent {
		args = append(args, "--agent")
	}
	appendStringArg(&args, "--models-dir", metadata.ModelsDir)
	appendStringArg(&args, "--models-preset", metadata.ModelsPreset)
	appendIntArg(&args, "--models-max", metadata.ModelsMax)
	appendOptionalBoolArg(&args, "--models-autoload", "--no-models-autoload", metadata.ModelsAutoload)
	return args, nil
}

func sdcppArguments(metadata catalog.RuntimeConfig, modelID string, host string, port string) ([]string, error) {
	_ = modelID
	modelPath := metadata.ImageModelPath()
	if modelPath == "" {
		return nil, fmt.Errorf("sd.cpp config has no image model path")
	}
	args := []string{
		"--listen-ip", host,
		"--listen-port", port,
		"--model", modelPath,
	}
	appendStringArg(&args, "--vae", metadata.SDVAE)
	appendStringArg(&args, "--diffusion-model", metadata.SDDiffusionModel)
	appendStringArg(&args, "--high-noise-diffusion-model", metadata.SDHighNoiseDiffusionModel)
	appendStringArg(&args, "--uncond-diffusion-model", metadata.SDUncondDiffusionModel)
	appendStringArg(&args, "--t5xxl", metadata.SDT5XXL)
	appendStringArg(&args, "--clip_l", firstNonEmpty(metadata.SDClipL, metadata.SDClip1))
	appendStringArg(&args, "--clip_g", firstNonEmpty(metadata.SDClipG, metadata.SDClip2))
	appendStringArg(&args, "--llm", metadata.SDLLM)
	appendStringArg(&args, "--llm-vision", metadata.SDLLMVision)
	appendStringArg(&args, "--clip-vision", metadata.SDClipVision)
	appendStringListArg(&args, "--embeddings-connector", metadata.SDEmbeddingsConnectors)
	appendStringArg(&args, "--control-net", metadata.SDControlNet)
	appendStringArg(&args, "--pulid-weights", metadata.SDPulidWeights)
	appendStringArg(&args, "--pulid-id-embedding", metadata.SDPulidIDEmbedding)
	appendFloatArg(&args, "--pulid-id-weight", metadata.SDPulidIDWeight)
	appendStringArg(&args, "--upscale-model", metadata.SDUpscaler)
	appendStringArg(&args, "--backend", metadata.SDBackend)
	appendStringArg(&args, "--params-backend", metadata.SDParamsBackend)
	appendStringListArg(&args, "--rpc-servers", metadata.SDRPCServers)
	appendIntArg(&args, "--max-vram", metadata.SDMaxVRAM)
	appendIntArg(&args, "--stream-layers", metadata.SDStreamLayers)
	appendStringListArg(&args, "--tensor-type-rules", metadata.SDTensorTypeRules)
	appendStringArg(&args, "--vae-format", metadata.SDVAEFormat)
	appendStringArg(&args, "--lora-model-dir", metadata.SDLoRAModelDir)
	appendStringArg(&args, "--upscaler-model-dir", metadata.SDHiresUpscalersDir)
	appendIntArg(&args, "--threads", metadata.SDThreads)
	if metadata.SDFlashAttention {
		args = append(args, "--fa")
	}
	if metadata.SDDiffusionFlashAttention {
		args = append(args, "--diffusion-fa")
	}
	if metadata.SDDiffusionConvDirect {
		args = append(args, "--diffusion-conv-direct")
	}
	if metadata.SDVAEConvDirect {
		args = append(args, "--vae-conv-direct")
	}
	if metadata.SDOffloadCPU {
		args = append(args, "--offload-to-cpu")
	}
	if metadata.SDVAECPU {
		args = append(args, "--vae-on-cpu")
	}
	if metadata.SDTiledVAE > 0 {
		tileSize := strconv.Itoa(metadata.SDTiledVAE) + "x" + strconv.Itoa(metadata.SDTiledVAE)
		args = append(args, "--vae-tiling", "--vae-tile-size", tileSize)
	}
	return args, nil
}

func appendStringArg(args *[]string, flag string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	*args = append(*args, flag, value)
}

func appendIntArg(args *[]string, flag string, value int) {
	if value == 0 {
		return
	}
	*args = append(*args, flag, strconv.Itoa(value))
}

func appendFloatArg(args *[]string, flag string, value float64) {
	if value == 0 {
		return
	}
	*args = append(*args, flag, strconv.FormatFloat(value, 'f', -1, 64))
}

func appendOptionalBoolArg(args *[]string, enabledFlag string, disabledFlag string, value *bool) {
	if value == nil {
		return
	}
	if *value {
		*args = append(*args, enabledFlag)
		return
	}
	*args = append(*args, disabledFlag)
}

func appendStringListArg(args *[]string, flag string, value any) {
	values := nativeStringValues(value)
	if len(values) == 0 {
		return
	}
	*args = append(*args, flag, strings.Join(values, ","))
}

func nativeStringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, nativeStringValues(item)...)
		}
		return values
	case float64:
		return []string{strconv.FormatFloat(typed, 'f', -1, 64)}
	case int:
		return []string{strconv.Itoa(typed)}
	default:
		return nil
	}
}

func positive(value int) int {
	if value > 0 {
		return value
	}
	return 0
}

func nonNegative(value int) int {
	if value >= 0 {
		return value
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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

func RuntimeArgumentsForTest(metadata catalog.RuntimeConfig, kind string) ([]string, error) {
	switch kind {
	case "llama":
		return llamaArguments(metadata, "model", "127.0.0.1", "5002")
	case "sdcpp":
		return sdcppArguments(metadata, "model", "127.0.0.1", "7860")
	default:
		return nil, fmt.Errorf("unknown native server kind %q", kind)
	}
}
