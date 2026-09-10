package analytics

import "testing"

func TestApplyResponseExtractsKoboldNativeGenerateCounts(t *testing.T) {
	event := Event{DurationMS: 6680}
	ApplyResponse(&event, "application/json", []byte(`{"results":[{"text":"hello","finish_reason":"stop","prompt_tokens":11,"completion_tokens":153}]}`))
	deriveTokenTotals(&event)

	if event.InputTokens != 11 || event.OutputTokens != 153 || event.TotalTokens != 164 {
		t.Fatalf("unexpected token counts %#v", event)
	}
}

func TestApplyEventStreamDataExtractsOAIResponsesUsage(t *testing.T) {
	event := Event{DurationMS: 1000}
	ApplyEventStreamData(&event, []byte(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":11,"output_tokens":153,"total_tokens":164}},"sequence_number":7}`))

	if event.InputTokens != 11 || event.OutputTokens != 153 || event.TotalTokens != 164 {
		t.Fatalf("unexpected token counts %#v", event)
	}
}

func TestApplyEventStreamDataExtractsAnthropicUsageAcrossEvents(t *testing.T) {
	event := Event{DurationMS: 1000}
	ApplyEventStreamData(&event, []byte(`{"type":"message_start","message":{"type":"message","role":"assistant","usage":{"input_tokens":11,"output_tokens":0}}}`))
	ApplyEventStreamData(&event, []byte(`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":153}}`))
	deriveTokenTotals(&event)

	if event.InputTokens != 11 || event.OutputTokens != 153 || event.TotalTokens != 164 {
		t.Fatalf("unexpected token counts %#v", event)
	}
}

func TestApplyResponseRejectsPlaceholderOllamaDurations(t *testing.T) {
	event := Event{DurationMS: 8590}
	ApplyResponse(&event, "application/json", []byte(`{"done":true,"prompt_eval_count":11,"prompt_eval_duration":1,"eval_count":195,"eval_duration":1}`))
	deriveTokenTotals(&event)

	if event.OutputTokens != 195 {
		t.Fatalf("unexpected output tokens %d", event.OutputTokens)
	}
	expected := 195 / 8.59
	if event.TokensPerSecond < expected-0.01 || event.TokensPerSecond > expected+0.01 {
		t.Fatalf("placeholder durations must fall back to wall clock, got %.2f", event.TokensPerSecond)
	}
}

func TestApplyResponseUsesRealOllamaDurations(t *testing.T) {
	event := Event{DurationMS: 8590}
	ApplyResponse(&event, "application/json", []byte(`{"done":true,"eval_count":195,"eval_duration":8490000000}`))

	expected := 195 / 8.49
	if event.TokensPerSecond < expected-0.01 || event.TokensPerSecond > expected+0.01 {
		t.Fatalf("unexpected tokens per second %.2f", event.TokensPerSecond)
	}
}

func TestApplyResponseIgnoresMissingArrayElement(t *testing.T) {
	event := Event{DurationMS: 1000}
	ApplyResponse(&event, "application/json", []byte(`{"results":[]}`))

	if event.InputTokens != 0 || event.OutputTokens != 0 || event.TotalTokens != 0 {
		t.Fatalf("empty results array should not produce counts %#v", event)
	}
}
