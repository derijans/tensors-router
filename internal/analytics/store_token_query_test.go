package analytics

import (
	"context"
	"testing"
	"time"
)

func recordTextRequestWithBytes(store *Store, now time.Time, modelID string, promptBytes int64, inputTokens int64, outputTokens int64, durationMS int64) {
	store.Record(Event{
		ModelID:      modelID,
		Section:      SectionLLM,
		BackendMode:  "kobold",
		Route:        "/v1/chat/completions",
		EventType:    EventTypeRequest,
		StatusCode:   200,
		Success:      true,
		StartedAt:    now.Add(-time.Duration(durationMS) * time.Millisecond),
		FinishedAt:   now,
		DurationMS:   durationMS,
		PromptBytes:  promptBytes,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	})
}

func TestTokenProfileSamplesRequirePromptBytes(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	recordTextRequestWithBytes(store, now, "llama", 800, 200, 50, 1000)
	recordTextRequestWithBytes(store, now, "llama", 0, 200, 50, 1000)

	samples, err := store.TokenProfileSamples(context.Background(), 24*time.Hour, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].Count != 1 {
		t.Fatalf("unbuffered row leaked into the token profile: %+v", samples)
	}
}

func TestTokenProfileSamplesGroupByModel(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	recordTextRequestWithBytes(store, now, "llama", 800, 200, 50, 1000)
	recordTextRequestWithBytes(store, now, "llama", 1600, 400, 100, 2000)
	recordTextRequestWithBytes(store, now, "mistral", 400, 100, 25, 500)

	samples, err := store.TokenProfileSamples(context.Background(), 24*time.Hour, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("samples = %d, want 2", len(samples))
	}
	byModel := map[string]TokenProfileSample{}
	for _, sample := range samples {
		byModel[sample.ModelID] = sample
	}
	if byModel["llama"].Count != 2 {
		t.Fatalf("llama count = %d, want 2", byModel["llama"].Count)
	}
	if byModel["mistral"].Count != 1 {
		t.Fatalf("mistral count = %d, want 1", byModel["mistral"].Count)
	}
}
