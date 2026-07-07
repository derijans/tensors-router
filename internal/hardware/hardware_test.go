package hardware

import (
	"context"
	"fmt"
	"runtime"
	"strings"
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

func TestReadVRAMAggregatesNVIDIA(t *testing.T) {
	info, ok := ReadVRAM(context.Background(), Detector{
		LookPath: commandExistsFor("nvidia-smi"),
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "nvidia-smi" {
				t.Fatalf("unexpected command %q", name)
			}
			if strings.Join(args, " ") != "--query-gpu=memory.used,memory.total --format=csv,noheader,nounits" {
				t.Fatalf("unexpected args %#v", args)
			}
			return []byte("1024, 8192\n2048, 16384\n"), nil
		},
	})

	if !ok {
		t.Fatal("expected vram info")
	}
	if info.UsedMB != 3072 || info.TotalMB != 24576 {
		t.Fatalf("unexpected aggregate %#v", info)
	}
	if info.UsedPercent != 12.5 {
		t.Fatalf("unexpected percent %.2f", info.UsedPercent)
	}
}

func TestReadVRAMAggregatesROCmCSV(t *testing.T) {
	info, ok := ReadVRAM(context.Background(), Detector{
		LookPath: commandExistsFor("rocm-smi"),
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "rocm-smi" {
				t.Fatalf("unexpected command %q", name)
			}
			if strings.Join(args, " ") != "--showmeminfo vram --csv" {
				t.Fatalf("unexpected args %#v", args)
			}
			return []byte("device,vram total memory (B),vram total used memory (B)\ncard0,8589934592,1073741824\ncard1,17179869184,2147483648\n"), nil
		},
	})

	if !ok {
		t.Fatal("expected vram info")
	}
	if info.UsedMB != 3072 || info.TotalMB != 24576 {
		t.Fatalf("unexpected rocm aggregate %#v", info)
	}
}

func TestReadVRAMFallsBackToROCmWhenNVIDIAOutputMalformed(t *testing.T) {
	info, ok := ReadVRAM(context.Background(), Detector{
		LookPath: func(name string) (string, error) {
			if name == "nvidia-smi" || name == "rocm-smi" {
				return name, nil
			}
			return "", fmt.Errorf("missing")
		},
		Run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "nvidia-smi" {
				return []byte("not,csv,enough\n"), nil
			}
			return []byte("device,vram total memory (B),vram total used memory (B)\ncard0,8589934592,1073741824\n"), nil
		},
	})

	if !ok {
		t.Fatal("expected rocm fallback")
	}
	if info.UsedMB != 1024 || info.TotalMB != 8192 {
		t.Fatalf("unexpected fallback aggregate %#v", info)
	}
}

func TestReadVRAMUnavailableWithoutCommands(t *testing.T) {
	_, ok := ReadVRAM(context.Background(), Detector{
		LookPath: func(string) (string, error) {
			return "", fmt.Errorf("missing")
		},
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("run should not be called")
			return nil, nil
		},
	})

	if ok {
		t.Fatal("expected unavailable vram")
	}
}

func TestReadVRAMRejectsMalformedOutput(t *testing.T) {
	_, ok := ReadVRAM(context.Background(), Detector{
		LookPath: commandExistsFor("nvidia-smi"),
		Run: func(context.Context, string, ...string) ([]byte, error) {
			return []byte("nope\n"), nil
		},
	})

	if ok {
		t.Fatal("expected malformed output to be unavailable")
	}
}

func commandExistsFor(names ...string) func(string) (string, error) {
	allowed := map[string]struct{}{}
	for _, name := range names {
		allowed[name] = struct{}{}
	}
	return func(name string) (string, error) {
		if _, ok := allowed[name]; ok {
			return name, nil
		}
		return "", fmt.Errorf("missing")
	}
}
