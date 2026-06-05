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
    if len(issues) != 2 {
        t.Fatalf("expected two issues, got %d", len(issues))
    }

    gotCodes := map[string]bool{}
    for _, issue := range issues {
        gotCodes[issue.Code] = true
    }
    if !gotCodes["unverified_option"] {
        t.Fatal("expected unverified_option issue")
    }
    if !gotCodes["unsupported_option"] {
        t.Fatal("expected unsupported_option issue")
    }
}
