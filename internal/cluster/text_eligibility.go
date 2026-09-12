package cluster

import "encoding/json"

func statesContextWindow(model Model) bool {
	return model.Capabilities.Context > 0
}

func servesRequestsConcurrently(model Model) bool {
	if optionIntAtLeast(model.Options, "parallel", 2) {
		return true
	}
	vllmRaw, ok := model.Options["vllm"]
	if !ok {
		return false
	}
	var vllmOptions struct {
		Settings map[string]json.RawMessage `json:"settings"`
	}
	if json.Unmarshal(vllmRaw, &vllmOptions) != nil {
		return false
	}
	return optionIntAtLeast(vllmOptions.Settings, "max_number_sequences", 2)
}

func TextGroupIneligibility(model Model) (string, bool) {
	if !model.HasLLM {
		return "model does not run as a text model", false
	}
	if !statesContextWindow(model) {
		return "model does not report a context window", false
	}
	if servesRequestsConcurrently(model) {
		return "model serves requests concurrently", false
	}
	return "", true
}

func TextGroupEligible(model Model) bool {
	_, eligible := TextGroupIneligibility(model)
	return eligible
}

func optionIntAtLeast(options map[string]json.RawMessage, key string, threshold int) bool {
	raw, ok := options[key]
	if !ok {
		return false
	}
	var value float64
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	return value >= float64(threshold)
}
