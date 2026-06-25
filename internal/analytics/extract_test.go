package analytics

import "testing"

func TestApplyResponseExtractsTextUsageAndSpeed(t *testing.T) {
	event := Event{DurationMS: 2000}
	ApplyResponse(&event, "application/json", []byte(`{
		"usage": {"prompt_tokens": 12, "completion_tokens": 8, "total_tokens": 20},
		"tokens_per_second": 11.5
	}`))

	if event.InputTokens != 12 || event.OutputTokens != 8 || event.TotalTokens != 20 {
		t.Fatalf("unexpected token counts %#v", event)
	}
	if event.TokensPerSecond != 11.5 {
		t.Fatalf("unexpected tokens per second %.2f", event.TokensPerSecond)
	}
}

func TestApplyResponseDerivesSpeedOnlyWhenOutputTokensAreReported(t *testing.T) {
	event := Event{DurationMS: 2000}
	ApplyResponse(&event, "application/json", []byte(`{"usage":{"completion_tokens":10}}`))
	if event.TokensPerSecond != 5 {
		t.Fatalf("expected derived token speed, got %.2f", event.TokensPerSecond)
	}

	empty := Event{DurationMS: 2000}
	ApplyResponse(&empty, "application/json", []byte(`{"choices":[{"message":{"content":"hello"}}]}`))
	if empty.TokensPerSecond != 0 {
		t.Fatalf("content-only response should not derive token speed, got %.2f", empty.TokensPerSecond)
	}
}

func TestApplyRequestExtractsImageMetadata(t *testing.T) {
	event := Event{Section: SectionImage}
	ApplyRequest(&event, "/sdapi/v1/img2img", []byte(`{"width":768,"height":512,"batch_size":2,"n_iter":3}`), "application/json")

	if event.ImageType != "img2img" {
		t.Fatalf("unexpected image type %q", event.ImageType)
	}
	if event.ImageWidth != 768 || event.ImageHeight != 512 || event.ImageCount != 6 {
		t.Fatalf("unexpected image metadata %#v", event)
	}
}

func TestApplyResponseExtractsAudioMetadata(t *testing.T) {
	event := Event{Section: SectionVoice}
	ApplyResponse(&event, "application/json", []byte(`{"duration_seconds":1.25,"audio_tokens":40}`))

	if event.AudioSeconds != 1.25 || event.AudioTokens != 40 {
		t.Fatalf("unexpected audio metadata %#v", event)
	}
}

func TestApplyResponseDoesNotPersistBodyContent(t *testing.T) {
	event := Event{}
	ApplyResponse(&event, "application/json", []byte(`{"choices":[{"message":{"content":"secret prompt-shaped text"}}],"usage":{"total_tokens":3}}`))

	if event.TotalTokens != 3 {
		t.Fatalf("expected metadata extraction, got %#v", event)
	}
	if event.ModelID == "secret prompt-shaped text" || event.Route == "secret prompt-shaped text" {
		t.Fatalf("body content leaked into metadata %#v", event)
	}
}
