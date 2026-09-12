package proxy

import (
	"encoding/json"

	"tensors-router/internal/cluster"
	"tensors-router/internal/schedulingcost"
)

func (service *Service) textRouteHint(profileNodeID string, profileModelID string, promptBytes int64, body []byte) cluster.RouteHint {
	if promptBytes <= 0 {
		return cluster.UnsizedRouteHint()
	}
	profile, ok := service.costSource.TokenProfile(profileNodeID, profileModelID)
	if !ok {
		return cluster.UnsizedRouteHint()
	}
	promptTokens, ok := profile.EstimatePromptTokens(promptBytes)
	if !ok {
		return cluster.UnsizedRouteHint()
	}
	requiredContext, ok := profile.RequiredContext(promptBytes, requestedOutputTokens(body), service.schedulingContextReserve)
	if !ok {
		return cluster.UnsizedRouteHint()
	}
	outputEstimate := requiredContext - promptTokens
	if outputEstimate < 0 {
		outputEstimate = 0
	}
	return cluster.RouteHint{
		Work:            schedulingcost.TextWork(float64(promptTokens), float64(outputEstimate)),
		RequiredContext: requiredContext,
	}
}

var outputTokenLimitKeys = []string{"max_tokens", "max_completion_tokens", "max_output_tokens", "n_predict"}

func requestedOutputTokens(body []byte) int {
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return 0
	}
	for _, key := range outputTokenLimitKeys {
		if value, ok := positiveJSONInt(raw, key); ok {
			return value
		}
	}
	return outputLimitFromOptions(raw)
}

func outputLimitFromOptions(raw map[string]json.RawMessage) int {
	optionsRaw, ok := raw["options"]
	if !ok {
		return 0
	}
	var options map[string]json.RawMessage
	if json.Unmarshal(optionsRaw, &options) != nil {
		return 0
	}
	value, _ := positiveJSONInt(options, "num_predict")
	return value
}

func positiveJSONInt(raw map[string]json.RawMessage, key string) (int, bool) {
	value, ok := raw[key]
	if !ok {
		return 0, false
	}
	var number float64
	if json.Unmarshal(value, &number) != nil || number <= 0 {
		return 0, false
	}
	return int(number), true
}

func requestIsStreaming(body []byte) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return false
	}
	value, ok := raw["stream"]
	if !ok {
		return false
	}
	var streaming bool
	if json.Unmarshal(value, &streaming) != nil {
		return false
	}
	return streaming
}
