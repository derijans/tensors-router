package hardware

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	GPUBackendCUDA    = "cuda"
	GPUBackendROCm    = "rocm"
	GPUBackendVulkan  = "vulkan"
	GPUBackendMetal   = "metal"
	GPUBackendCPU     = "cpu"
	GPUBackendUnknown = "unknown"
)

type Info struct {
	MaxThreads  int    `json:"max_threads"`
	GPUBackend  string `json:"gpu_backend"`
	GPUCount    int    `json:"gpu_count"`
	CUDAVersion string `json:"cuda_version,omitempty"`
	ROCmVersion string `json:"rocm_version,omitempty"`
}

type Source interface {
	Info(context.Context) Info
}

type Cache struct {
	mu       sync.Mutex
	ttl      time.Duration
	expires  time.Time
	current  Info
	detector Detector
}

type StaticSource struct {
	Value Info
}

type Detector struct {
	LookPath func(string) (string, error)
	Run      func(context.Context, string, ...string) ([]byte, error)
}

func NewCache() *Cache {
	return &Cache{
		ttl:      5 * time.Second,
		detector: defaultDetector(),
	}
}

func NewStatic(value Info) StaticSource {
	if value.MaxThreads <= 0 {
		value.MaxThreads = runtime.NumCPU()
	}
	if value.GPUBackend == "" {
		value.GPUBackend = GPUBackendUnknown
	}
	return StaticSource{Value: value}
}

func (source StaticSource) Info(context.Context) Info {
	return normalize(source.Value)
}

func (cache *Cache) Info(ctx context.Context) Info {
	if cache == nil {
		return Detect(ctx, defaultDetector())
	}
	now := time.Now()
	cache.mu.Lock()
	if now.Before(cache.expires) && cache.current.MaxThreads > 0 {
		value := cache.current
		cache.mu.Unlock()
		return value
	}
	detector := cache.detector
	cache.mu.Unlock()

	value := Detect(ctx, detector)

	cache.mu.Lock()
	cache.current = value
	cache.expires = now.Add(cache.ttl)
	cache.mu.Unlock()
	return value
}

func Detect(ctx context.Context, detector Detector) Info {
	info := Info{
		MaxThreads: runtime.NumCPU(),
		GPUBackend: GPUBackendUnknown,
	}
	if runtime.GOOS == "darwin" {
		info.GPUBackend = GPUBackendMetal
		info.GPUCount = 1
		return normalize(info)
	}
	if detector.LookPath == nil || detector.Run == nil {
		detector = defaultDetector()
	}
	if output, ok := probeCommand(ctx, detector, "nvidia-smi", "-L"); ok {
		info.GPUBackend = GPUBackendCUDA
		info.GPUCount = countNonEmptyLines(output)
		if versionOutput, versionOK := probeCommand(ctx, detector, "nvidia-smi"); versionOK {
			info.CUDAVersion = detectedRuntimeVersion(string(versionOutput), "CUDA Version:")
		}
		return normalize(info)
	}
	if output, ok := probeCommand(ctx, detector, "rocm-smi", "--showid"); ok {
		info.GPUBackend = GPUBackendROCm
		info.GPUCount = countLinesContaining(output, "GPU")
		return normalize(info)
	}
	if output, ok := probeCommand(ctx, detector, "rocminfo"); ok {
		info.GPUBackend = GPUBackendROCm
		info.ROCmVersion = detectedRuntimeVersion(string(output), "Runtime Version:")
		return normalize(info)
	}
	if _, ok := probeCommand(ctx, detector, "vulkaninfo"); ok {
		info.GPUBackend = GPUBackendVulkan
		return normalize(info)
	}
	return normalize(info)
}

func detectedRuntimeVersion(output string, marker string) string {
	for _, line := range strings.Split(output, "\n") {
		index := strings.Index(line, marker)
		if index < 0 {
			continue
		}
		value := strings.TrimSpace(line[index+len(marker):])
		for _, field := range strings.Fields(value) {
			if strings.Count(field, ".") >= 1 {
				return strings.Trim(field, " ,;")
			}
		}
	}
	return ""
}

func defaultDetector() Detector {
	return Detector{
		LookPath: exec.LookPath,
		Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
	}
}

func commandExists(detector Detector, name string) bool {
	_, err := detector.LookPath(name)
	return err == nil
}

func probeCommand(ctx context.Context, detector Detector, name string, args ...string) ([]byte, bool) {
	if !commandExists(detector, name) {
		return nil, false
	}
	output, err := runWithTimeout(ctx, detector, name, args...)
	return output, err == nil
}

func runWithTimeout(ctx context.Context, detector Detector, name string, args ...string) ([]byte, error) {
	runCtx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	return detector.Run(runCtx, name, args...)
}

func countNonEmptyLines(output []byte) int {
	lines := bytes.Split(output, []byte{'\n'})
	count := 0
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) > 0 {
			count++
		}
	}
	return count
}

func countLinesContaining(output []byte, needle string) int {
	lines := strings.Split(string(output), "\n")
	count := 0
	for _, line := range lines {
		if strings.Contains(line, needle) {
			count++
		}
	}
	return count
}

func normalize(info Info) Info {
	if info.MaxThreads <= 0 {
		info.MaxThreads = runtime.NumCPU()
	}
	if info.GPUBackend == "" {
		info.GPUBackend = GPUBackendUnknown
	}
	if info.GPUCount < 0 {
		info.GPUCount = 0
	}
	return info
}
