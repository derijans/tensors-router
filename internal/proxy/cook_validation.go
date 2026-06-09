package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/hardware"
	"tensors-router/internal/siteapi"
)

type cookNodeFacts struct {
	backendMode string
	hardware    hardware.Info
	models      []cluster.Model
}

type cookValidationError struct {
	message string
	issues  []cook.ValidationIssue
}

func (err cookValidationError) Error() string {
	return err.message
}

func validationIssues(err error) ([]cook.ValidationIssue, bool) {
	var validation cookValidationError
	if errors.As(err, &validation) {
		return validation.issues, true
	}
	return nil, false
}

func (service *Service) validateCookGroups(ctx context.Context, groups []cookGroup, options cook.Options) ([]cook.ValidationIssue, error) {
	facts, err := service.cookNodeFacts(ctx, groups)
	if err != nil {
		return nil, err
	}
	issues := make([]cook.ValidationIssue, 0)
	for _, group := range groups {
		fact := facts[group.nodeID]
		groupOptions := cook.FilterOptionsForKinds(options, group.components)
		issues = append(issues, validateKoboldMix(group, fact)...)
		issues = append(issues, validateThreadBudget(group, fact, groupOptions)...)
		issues = append(issues, validateCUDAOptions(group, fact, groupOptions)...)
		issues = append(issues, validateOptionSupport(group, fact, groupOptions)...)
		issues = append(issues, validateUnknownGPU(group, fact, groupOptions)...)
		issues = append(issues, validateUnknownGPUCount(group, fact, groupOptions)...)
	}
	if hasValidationErrors(issues) {
		return issues, cookValidationError{message: validationMessage(issues), issues: issues}
	}
	return issues, nil
}

func (service *Service) cookNodeFacts(ctx context.Context, groups []cookGroup) (map[string]cookNodeFacts, error) {
	result := map[string]cookNodeFacts{}
	for _, group := range groups {
		if _, ok := result[group.nodeID]; ok {
			continue
		}
		if group.local {
			models, err := service.localClusterModels()
			if err != nil {
				return nil, err
			}
			result[group.nodeID] = cookNodeFacts{
				backendMode: service.backendMode,
				hardware:    service.hardware.Info(ctx),
				models:      models,
			}
			continue
		}
		var node siteapi.NodeInventory
		if err := service.clusterClient.JSON(ctx, http.MethodGet, group.nodeURL, "/router/v1/node/site/inventory", nil, &node); err != nil {
			return nil, err
		}
		backendMode := strings.TrimSpace(node.BackendMode)
		if backendMode == "" {
			backendMode = BackendModeKobold
		}
		result[group.nodeID] = cookNodeFacts{
			backendMode: backendMode,
			hardware:    node.Hardware,
			models:      node.Models,
		}
	}
	return result, nil
}

func validateKoboldMix(group cookGroup, fact cookNodeFacts) []cook.ValidationIssue {
	if fact.backendMode != BackendModeKobold {
		return nil
	}
	if !groupHasKind(group, cook.KindImage) || !groupHasKind(group, cook.KindEmbeddings) {
		return nil
	}
	return []cook.ValidationIssue{{
		Severity: "error",
		Code:     "kobold_image_embeddings_mix",
		Message:  "Kobold nodes cannot cook image and embeddings into the same config.",
		NodeID:   group.nodeID,
	}}
}

func validateThreadBudget(group cookGroup, fact cookNodeFacts, options cook.Options) []cook.ValidationIssue {
	threadFields := selectedThreadFields(group, fact, options)
	issues := make([]cook.ValidationIssue, 0, len(threadFields))
	for _, field := range threadFields {
		if field.value <= 0 {
			continue
		}
		if fact.hardware.MaxThreads <= 0 {
			issues = append(issues, cook.ValidationIssue{
				Severity: "warning",
				Code:     "thread_budget_unknown",
				Message:  fmt.Sprintf("Thread budget for %q could not be inferred for this node.", field.key),
				NodeID:   group.nodeID,
				Field:    field.key,
			})
			continue
		}
		if field.value <= fact.hardware.MaxThreads {
			continue
		}
		issues = append(issues, cook.ValidationIssue{
			Severity: "error",
			Code:     "thread_budget_exceeded",
			Message:  fmt.Sprintf("Selected %q assigns %d threads on a node with %d logical CPUs.", field.key, field.value, fact.hardware.MaxThreads),
			NodeID:   group.nodeID,
			Field:    field.key,
		})
	}
	return issues
}

func validateCUDAOptions(group cookGroup, fact cookNodeFacts, options cook.Options) []cook.ValidationIssue {
	if fact.hardware.GPUBackend != hardware.GPUBackendROCm {
		return nil
	}
	issues := make([]cook.ValidationIssue, 0)
	for key, value := range selectedOptions(group, fact, options) {
		if cook.IsCUDAOnlyOption(key) && rawTruthy(value) {
			issues = append(issues, cook.ValidationIssue{
				Severity: "error",
				Code:     "cuda_on_rocm",
				Message:  fmt.Sprintf("CUDA-only option %q is enabled on a ROCm node.", key),
				NodeID:   group.nodeID,
				Field:    key,
			})
		}
	}
	return issues
}

func validateOptionSupport(group cookGroup, fact cookNodeFacts, options cook.Options) []cook.ValidationIssue {
	issues := make([]cook.ValidationIssue, 0)
	for key := range selectedOptions(group, fact, options) {
		definition, ok := cook.OptionDefinitionForKey(key)
		if !ok || !definition.Known {
			continue
		}
		if len(definition.Backends) > 0 && !backendSupported(fact.backendMode, definition.Backends) {
			issues = append(issues, cook.ValidationIssue{
				Severity: "warning",
				Code:     "unsupported_option",
				Message:  fmt.Sprintf("Option %q is not marked as supported by %s.", key, fact.backendMode),
				NodeID:   group.nodeID,
				Field:    key,
			})
		}
	}
	return issues
}

func validateUnknownGPU(group cookGroup, fact cookNodeFacts, options cook.Options) []cook.ValidationIssue {
	if fact.hardware.GPUBackend != "" && fact.hardware.GPUBackend != hardware.GPUBackendUnknown {
		return nil
	}
	for key, value := range selectedOptions(group, fact, options) {
		if gpuOptionKey(key) && rawTruthy(value) {
			return []cook.ValidationIssue{{
				Severity: "warning",
				Code:     "gpu_backend_unknown",
				Message:  "GPU backend could not be inferred for this node.",
				NodeID:   group.nodeID,
				Field:    key,
			}}
		}
	}
	return nil
}

func validateUnknownGPUCount(group cookGroup, fact cookNodeFacts, options cook.Options) []cook.ValidationIssue {
	if fact.hardware.GPUBackend == "" || fact.hardware.GPUBackend == hardware.GPUBackendUnknown {
		return nil
	}
	if fact.hardware.GPUCount > 0 {
		return nil
	}
	for key, value := range selectedOptions(group, fact, options) {
		if highGPUOption(key, value) {
			return []cook.ValidationIssue{{
				Severity: "warning",
				Code:     "gpu_count_unknown",
				Message:  "GPU count could not be inferred for high GPU or offload settings.",
				NodeID:   group.nodeID,
				Field:    key,
			}}
		}
	}
	return nil
}

type selectedThreadField struct {
	key   string
	value int
}

func selectedThreadFields(group cookGroup, fact cookNodeFacts, options cook.Options) []selectedThreadField {
	fields := make([]selectedThreadField, 0, 4)
	for key, value := range selectedOptions(group, fact, options) {
		if !threadCountOption(key) {
			continue
		}
		if count, ok := rawInt(value); ok {
			fields = append(fields, selectedThreadField{key: key, value: count})
		}
	}
	sort.Slice(fields, func(left, right int) bool {
		return fields[left].key < fields[right].key
	})
	return fields
}

func selectedOptions(group cookGroup, fact cookNodeFacts, options cook.Options) cook.Options {
	selected := cook.Options{}
	for _, component := range group.components {
		if component.Source != cook.SourceConfig {
			continue
		}
		model, ok := componentModel(fact.models, component)
		if !ok {
			continue
		}
		for key, value := range model.Options {
			selected[key] = value
		}
	}
	for key, value := range options {
		selected[key] = value
	}
	return selected
}

func componentModel(models []cluster.Model, component cook.Component) (cluster.Model, bool) {
	switch component.Kind {
	case cook.KindImage:
		imageID := strings.TrimSpace(component.ImageID)
		if imageID == "" {
			imageID = strings.TrimSpace(component.ModelID)
		}
		for _, model := range models {
			if model.ImageID == imageID || model.PublicImageID == imageID {
				return model, true
			}
		}
	default:
		modelID := strings.TrimSpace(component.ModelID)
		for _, model := range models {
			if model.LocalID == modelID || model.PublicID == modelID {
				return model, true
			}
		}
	}
	return cluster.Model{}, false
}

func rawInt(value json.RawMessage) (int, bool) {
	if len(value) == 0 {
		return 0, false
	}
	var number int
	if err := json.Unmarshal(value, &number); err == nil {
		return number, true
	}
	var floating float64
	if err := json.Unmarshal(value, &floating); err == nil {
		return int(floating), true
	}
	var text string
	if err := json.Unmarshal(value, &text); err == nil {
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(text), "%d", &parsed); err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func rawTruthy(value json.RawMessage) bool {
	if len(value) == 0 {
		return false
	}
	var boolean bool
	if err := json.Unmarshal(value, &boolean); err == nil {
		return boolean
	}
	var number float64
	if err := json.Unmarshal(value, &number); err == nil {
		return number != 0
	}
	var text string
	if err := json.Unmarshal(value, &text); err == nil {
		return strings.TrimSpace(text) != ""
	}
	return strings.TrimSpace(string(value)) != "" && !strings.EqualFold(strings.TrimSpace(string(value)), "null")
}

func groupHasKind(group cookGroup, kind string) bool {
	for _, component := range group.components {
		if component.Kind == kind {
			return true
		}
	}
	return false
}

func backendSupported(backend string, values []string) bool {
	for _, value := range values {
		if value == backend {
			return true
		}
	}
	return false
}

func threadCountOption(key string) bool {
	definition, ok := cook.OptionDefinitionForKey(key)
	if ok {
		return definition.ValueType == cook.ValueNumber && strings.HasSuffix(definition.Key, "threads")
	}
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(key)), "threads")
}

func gpuOptionKey(key string) bool {
	switch key {
	case "gpulayers", "tensor_split", "maingpu", "usecuda", "usecublas", "embeddingsgpu", "sdclipgpu", "sdflashattention":
		return true
	default:
		return strings.Contains(strings.ToLower(key), "gpu") || strings.Contains(strings.ToLower(key), "cuda")
	}
}

func highGPUOption(key string, value json.RawMessage) bool {
	switch key {
	case "gpulayers", "maingpu":
		number, ok := rawInt(value)
		return ok && number > 0
	case "tensor_split", "usecuda", "usecublas", "embeddingsgpu", "sdclipgpu", "sdflashattention":
		return rawTruthy(value)
	default:
		return gpuOptionKey(key) && rawTruthy(value)
	}
}

func hasValidationErrors(issues []cook.ValidationIssue) bool {
	for _, issue := range issues {
		if issue.Severity == "error" {
			return true
		}
	}
	return false
}

func validationMessage(issues []cook.ValidationIssue) string {
	messages := make([]string, 0)
	for _, issue := range issues {
		if issue.Severity == "error" {
			messages = append(messages, issue.Message)
		}
	}
	if len(messages) == 0 {
		return "validation failed"
	}
	return strings.Join(messages, "; ")
}

func observedOptions(nodes []siteapi.NodeInventory, models []cluster.Model) []cook.OptionDefinition {
	values := make([]cook.Options, 0)
	for _, model := range models {
		values = appendCookOptions(values, model.Options)
	}
	for _, node := range nodes {
		for _, model := range node.Models {
			values = appendCookOptions(values, model.Options)
		}
	}
	return cook.ObservedOptions(values)
}

func appendCookOptions(values []cook.Options, options map[string]json.RawMessage) []cook.Options {
	if len(options) == 0 {
		return values
	}
	copied := cook.Options{}
	for key, value := range options {
		copied[key] = value
	}
	return append(values, copied)
}
