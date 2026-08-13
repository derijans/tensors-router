package vllm

import (
	"context"
	"reflect"
	"testing"
)

func TestSystemDetectorRequiresWorkingAcceleratorRuntimeCommands(t *testing.T) {
	workingCommands := map[string]bool{}
	detector := SystemDetector{
		operatingSystem:  "linux",
		architecture:     "amd64",
		deviceExists:     func(path string) bool { return path == "/dev/nvidiactl" || path == "/dev/kfd" },
		intelGPUDetected: func() bool { return true },
		prerequisiteCommand: func(_ context.Context, name string, _ ...string) bool {
			return workingCommands[name]
		},
	}
	detection, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(detection.Devices, []string{"xpu", "rocm", "cuda", "cpu"}) {
		t.Fatalf("unexpected devices %#v", detection.Devices)
	}
	for _, prerequisite := range []string{"nvidia_driver", "rocm_driver", "intel_gpu"} {
		if detection.Prerequisites[prerequisite] {
			t.Fatalf("device presence satisfied %s without a working runtime command", prerequisite)
		}
	}

	workingCommands["nvidia-smi"] = true
	workingCommands["rocminfo"] = true
	workingCommands["sycl-ls"] = true
	detection, err = detector.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, prerequisite := range []string{"nvidia_driver", "rocm_driver", "intel_gpu"} {
		if !detection.Prerequisites[prerequisite] {
			t.Fatalf("working runtime command did not satisfy %s", prerequisite)
		}
	}
}

func TestSystemDetectorRecognizesAlternateROCmAndIntelTools(t *testing.T) {
	for _, commands := range []map[string]bool{
		{"amd-smi": true, "xpu-smi": true},
		{"rocm-smi": true, "sycl-ls": true},
	} {
		detector := SystemDetector{
			operatingSystem:  "linux",
			architecture:     "amd64",
			deviceExists:     func(path string) bool { return path == "/dev/kfd" },
			intelGPUDetected: func() bool { return true },
			prerequisiteCommand: func(_ context.Context, name string, _ ...string) bool {
				return commands[name]
			},
		}
		detection, err := detector.Detect(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !detection.Prerequisites["rocm_driver"] || !detection.Prerequisites["intel_gpu"] {
			t.Fatalf("alternate vendor tools were not recognized: %#v", detection.Prerequisites)
		}
	}
}
