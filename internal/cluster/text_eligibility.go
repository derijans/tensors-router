package cluster

import "encoding/json"

// TextGroupEligible reports whether a model may join, or remain in, a text
// routing group. Checked in three places: the site handler never offers an
// ineligible candidate, saving a group rejects one, and modelForMemberLocked
// applies it again at selection time so a member edited to become concurrent
// or to drop its context size after being grouped is skipped rather than
// routed to.
//
//   - Not concurrent: a config serving requests concurrently (llama.cpp
//     parallel, or vLLM max_number_sequences, both > 1) runs outside the
//     one-at-a-time discipline the router queue assumes, so putting it behind
//     a serializing queue would be a throughput regression.
//   - A stated context window: a model that does not report one cannot be
//     gated, and a model that cannot be gated has no business in a group.
func TextGroupEligible(model Model) bool {
	if !model.HasLLM {
		return false
	}
	if model.Capabilities.Context <= 0 {
		return false
	}
	if optionIntAtLeast(model.Options, "parallel", 2) {
		return false
	}
	if vllmRaw, ok := model.Options["vllm"]; ok {
		var vllmOptions struct {
			Settings map[string]json.RawMessage `json:"settings"`
		}
		if json.Unmarshal(vllmRaw, &vllmOptions) == nil && optionIntAtLeast(vllmOptions.Settings, "max_number_sequences", 2) {
			return false
		}
	}
	return true
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
