package proxy

import (
	"encoding/json"
	"testing"

	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/hardware"
)

func TestValidateUnknownGPUCountWarnsWhenBackendKnownAndGPUCountUnknown(t *testing.T) {
	group := cookGroup{
		components: []cook.Component{{
			Kind:    cook.KindText,
			Source:  cook.SourceConfig,
			ModelID: "text-model",
		}},
	}
	fact := cookNodeFacts{
		backendMode: "llama_sdcpp",
		hardware: hardware.Info{
			GPUBackend: hardware.GPUBackendVulkan,
			GPUCount:   0,
		},
		models: []cluster.Model{{
			LocalID: "text-model",
			Options: map[string]json.RawMessage{
				"usecuda": json.RawMessage("true"),
			},
		}},
	}

	issues := validateUnknownGPUCount(group, fact, nil)
	if len(issues) != 1 {
		t.Fatalf("expected one issue, got %d", len(issues))
	}
	if issues[0].Code != "gpu_count_unknown" {
		t.Fatalf("expected gpu_count_unknown, got %q", issues[0].Code)
	}
}

func TestValidateUnknownGPUCountSkipsWhenBackendUnknown(t *testing.T) {
	group := cookGroup{
		components: []cook.Component{{
			Kind:    cook.KindText,
			Source:  cook.SourceConfig,
			ModelID: "text-model",
		}},
	}
	fact := cookNodeFacts{
		backendMode: "kobold",
		hardware: hardware.Info{
			GPUBackend: hardware.GPUBackendUnknown,
			GPUCount:   0,
		},
		models: []cluster.Model{{
			LocalID: "text-model",
			Options: map[string]json.RawMessage{
				"usecuda": json.RawMessage("true"),
			},
		}},
	}

	issues := validateUnknownGPUCount(group, fact, nil)
	if len(issues) != 0 {
		t.Fatalf("expected no issues for unknown backend, got %d", len(issues))
	}
}

func TestValidateOptionSupportIncludesSourceConfigOptions(t *testing.T) {
	group := cookGroup{
		components: []cook.Component{{
			Kind:    cook.KindText,
			Source:  cook.SourceConfig,
			ModelID: "text-model",
		}},
	}
	fact := cookNodeFacts{
		backendMode: "llama_sdcpp",
		hardware: hardware.Info{
			GPUBackend: hardware.GPUBackendVulkan,
			GPUCount:   1,
		},
		models: []cluster.Model{{
			LocalID: "text-model",
			Options: map[string]json.RawMessage{
				"unknown_flag": json.RawMessage("true"),
				"usecublas":    json.RawMessage("true"),
			},
		}},
	}

	issues := validateOptionSupport(group, fact, nil)
	if len(issues) != 1 {
		t.Fatalf("expected one issue, got %d", len(issues))
	}

	gotCodes := map[string]bool{}
	for _, issue := range issues {
		gotCodes[issue.Code] = true
	}
	if gotCodes["unverified_option"] {
		t.Fatal("unknown observed options should not emit unverified_option")
	}
	if !gotCodes["unsupported_option"] {
		t.Fatal("expected unsupported_option issue")
	}
}

func TestValidateOptionSupportCatalogsDocumentedBackendOptions(t *testing.T) {
	group := cookGroup{
		components: []cook.Component{{
			Kind:    cook.KindImage,
			Source:  cook.SourceConfig,
			ModelID: "image-model",
		}},
	}
	fact := cookNodeFacts{
		backendMode: "kobold",
		hardware: hardware.Info{
			GPUBackend: hardware.GPUBackendCUDA,
			GPUCount:   1,
		},
		models: []cluster.Model{{
			LocalID: "image-model",
			Options: map[string]json.RawMessage{
				"baseconfig":      json.RawMessage(`"base.kcpps"`),
				"sdvaedevice":     json.RawMessage(`"main"`),
				"sdmaingpu":       json.RawMessage(`"0"`),
				"ttsmodel":        json.RawMessage(`"voice.gguf"`),
				"sampling_method": json.RawMessage(`"euler"`),
			},
		}},
	}

	issues := validateOptionSupport(group, fact, nil)
	for _, issue := range issues {
		if issue.Code == "unverified_option" {
			t.Fatalf("documented option emitted unverified warning: %#v", issue)
		}
	}
}
