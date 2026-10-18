package catalog

import (
	"encoding/json"
	"strings"

	"tensors-router/internal/backendmode"
)

const routerOnlyKeyPrefix = "router_"

type RequestProfilePolicy struct {
	InjectGenerationDefaults      bool
	ShareRuntimeByLaunchArguments bool
	DefaultBackendMode            string
}

func (policy RequestProfilePolicy) injectsGenerationDefaults(values map[string]json.RawMessage) bool {
	if !policy.InjectGenerationDefaults || configNamesImageModel(values) {
		return false
	}
	switch policy.backendFamily(values) {
	case backendmode.Kobold, backendmode.LlamaSDCPP:
		return true
	default:
		return false
	}
}

func (policy RequestProfilePolicy) backendFamily(values map[string]json.RawMessage) string {
	var configured string
	_ = json.Unmarshal(values["backend_mode"], &configured)
	if family := backendmode.Normalize(configured); family != "" {
		return family
	}
	return backendmode.Normalize(policy.DefaultBackendMode)
}

func (policy RequestProfilePolicy) RouterOwnsGenerationDefaults(content []byte) bool {
	values, err := decodeConfigObject(content)
	return err == nil && policy.injectsGenerationDefaults(values) && len(parseGenerationDefaultsLikeKoboldCpp(values[GenerationDefaultsKey])) > 0
}

func configNamesImageModel(values map[string]json.RawMessage) bool {
	for _, field := range imageModelFields {
		var path string
		if json.Unmarshal(values[field], &path) == nil && strings.TrimSpace(path) != "" {
			return true
		}
	}
	return portableModelField(values, imageModelFields...)
}

func launchFingerprintValues(values map[string]json.RawMessage, injectsGenerationDefaults bool) map[string]json.RawMessage {
	launch := make(map[string]json.RawMessage, len(values))
	for key, value := range values {
		if strings.HasPrefix(key, routerOnlyKeyPrefix) {
			continue
		}
		if injectsGenerationDefaults && (key == GenerationDefaultsKey || key == GenerationDefaultsOverwriteKey) {
			continue
		}
		launch[key] = value
	}
	return launch
}
