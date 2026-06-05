package hardware

import (
	"context"
	"fmt"
	"runtime"
	"testing"
)

func TestDetectUsesLogicalCPUFallback(t *testing.T) {
	info := Detect(context.Background(), Detector{
		LookPath: func(string) (string, error) {
			return "", fmt.Errorf("missing")
		},
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("run should not be called")
			return nil, nil
		},
	})
	if info.MaxThreads != runtime.NumCPU() {
		t.Fatalf("expected logical CPU count, got %#v", info)
	}
	if runtime.GOOS != "darwin" && info.GPUBackend != GPUBackendUnknown {
		t.Fatalf("expected unknown GPU backend, got %#v", info)
	}
}

func TestDetectPrefersCUDA(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin reports metal before probing command line tools")
	}
	info := Detect(context.Background(), Detector{
		LookPath: func(name string) (string, error) {
			if name == "nvidia-smi" {
				return name, nil
			}
			return "", fmt.Errorf("missing")
		},
		Run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name != "nvidia-smi" {
				t.Fatalf("unexpected command %q", name)
			}
			return []byte("GPU 0\nGPU 1\n"), nil
		},
	})
	if info.GPUBackend != GPUBackendCUDA || info.GPUCount != 2 {
		t.Fatalf("unexpected cuda detection %#v", info)
	}
}
