package proxy

import (
	"encoding/json"

	"tensors-router/internal/cluster"
	"tensors-router/internal/schedulingcost"
)

// textRouteHint prices a text request before it is dispatched, reusing the
// token profile fitted for one representative (node, model) — the local node
// and the requested public model id — even though the request may end up
// served by a different member of the same routing group. A bytes-per-token
// ratio is close to a tokenizer property, so this is a deliberate
// approximation rather than pricing every candidate's own profile up front,
// which would require restructuring group selection to price per candidate
// before a candidate is chosen. promptBytes is the caller's own measurement of
// the request's size — the raw client body for a buffered request,
// Content-Length for a streaming one whose body was never held in memory, and
// 0 for a chunked request with neither. A zero promptBytes, or no measured
// token profile for the representative pair, yields an empty hint: an
// unpriced request skips cost ordering and the context gate entirely rather
// than being priced on a guess.
func (service *Service) textRouteHint(nodeID string, modelID string, promptBytes int64, body []byte) cluster.RouteHint {
	if promptBytes <= 0 {
		return cluster.RouteHint{}
	}
	profile, ok := service.costSource.TokenProfile(nodeID, modelID)
	if !ok {
		return cluster.RouteHint{}
	}
	promptTokens, ok := profile.EstimatePromptTokens(promptBytes)
	if !ok {
		return cluster.RouteHint{}
	}
	requiredContext, ok := profile.RequiredContext(promptBytes, requestedOutputTokens(body), service.schedulingContextReserve)
	if !ok {
		return cluster.RouteHint{}
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

// requestedOutputTokens reads every spelling of an output-token limit this
// project's supported clients send: OpenAI's max_tokens and its
// max_completion_tokens replacement, the Responses API's max_output_tokens,
// llama.cpp's n_predict, and Ollama's options.num_predict. The first one
// present wins; a body that names none returns 0, which RequiredContext then
// falls back to the model's own measured mean for.
func requestedOutputTokens(body []byte) int {
	var raw map[string]json.RawMessage
	if json.Unmarshal(body, &raw) != nil {
		return 0
	}
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens", "n_predict"} {
		if value, ok := jsonIntSelector(raw, key); ok {
			return value
		}
	}
	optionsRaw, ok := raw["options"]
	if !ok {
		return 0
	}
	var options map[string]json.RawMessage
	if json.Unmarshal(optionsRaw, &options) != nil {
		return 0
	}
	value, _ := jsonIntSelector(options, "num_predict")
	return value
}

func jsonIntSelector(raw map[string]json.RawMessage, key string) (int, bool) {
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

// requestIsStreaming reports the client's stream flag. A request that cannot
// be parsed, or that never sets the flag, is treated as non-streaming, which
// is the side that stays offloadable — the safe default, since an
// unrecognised streaming request that gets offloaded is a wasted lease at
// worst, while one wrongly held back from offload is a request that never
// gets the benefit this exists to provide.
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
