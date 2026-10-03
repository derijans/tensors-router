package analytics

import (
	"context"
	"io"
	"math"
	"testing"
	"time"
)

func TestApplyEventStreamDataReadsLlamaPromptSpeed(t *testing.T) {
	event := Event{}
	ApplyEventStreamData(&event, []byte(`{"choices":[{"finish_reason":"stop","index":0,"delta":{}}],"timings":{"cache_n":20,"prompt_n":512,"prompt_ms":400,"prompt_per_second":1280,"predicted_n":40,"predicted_ms":2927.63,"predicted_per_second":13.66}}`))

	if event.PromptTokensPS != 1280 {
		t.Fatalf("llama.cpp prompt speed was not read, got %.2f", event.PromptTokensPS)
	}
}

func TestApplyEventStreamDataRejectsLlamaPromptSpeedWithoutMeasurablePrefill(t *testing.T) {
	event := Event{}
	ApplyEventStreamData(&event, []byte(`{"timings":{"prompt_n":1,"prompt_ms":0.2,"prompt_per_second":5000,"predicted_n":40,"predicted_ms":2000,"predicted_per_second":20}}`))

	if event.PromptTokensPS != 0 {
		t.Fatalf("a prefill shorter than a millisecond is no measurement, got %.2f", event.PromptTokensPS)
	}
}

func TestApplyResponseDerivesOllamaPromptSpeed(t *testing.T) {
	event := Event{}
	ApplyResponse(&event, "application/json", []byte(`{"done":true,"prompt_eval_count":300,"prompt_eval_duration":250000000,"eval_count":20,"eval_duration":1000000000}`))

	if math.Abs(event.PromptTokensPS-1200) > 0.01 {
		t.Fatalf("unexpected ollama prompt speed %.2f", event.PromptTokensPS)
	}
}

func TestApplyResponseRejectsPlaceholderOllamaPromptDuration(t *testing.T) {
	event := Event{}
	ApplyResponse(&event, "application/json", []byte(`{"done":true,"prompt_eval_count":300,"prompt_eval_duration":1,"eval_count":20,"eval_duration":1}`))

	if event.PromptTokensPS != 0 {
		t.Fatalf("placeholder prompt duration must not produce a speed, got %.2f", event.PromptTokensPS)
	}
}

func TestApplyEventStreamDataReadsRouterReportedPromptSpeed(t *testing.T) {
	event := Event{}
	ApplyEventStreamData(&event, []byte(`{"usage":{"prompt_tokens":5,"completion_tokens":20,"prompt_tokens_per_second":812.5}}`))

	if event.PromptTokensPS != 812.5 {
		t.Fatalf("usage prompt speed was not read, got %.2f", event.PromptTokensPS)
	}
}

func TestDeriveTotalsEstimatesPromptSpeedFromWorkStartToFirstToken(t *testing.T) {
	requestArrived := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	event := Event{
		StartedAt:     requestArrived,
		WorkStartedAt: requestArrived.Add(30 * time.Second),
		TTFTMS:        (30*time.Second + 500*time.Millisecond).Milliseconds(),
		InputTokens:   2000,
	}
	DeriveTotals(&event)

	if math.Abs(event.PromptTokensPS-4000) > 0.01 {
		t.Fatalf("prefill must exclude the model load before work started, got %.2f", event.PromptTokensPS)
	}
}

func TestDeriveTotalsLeavesPromptSpeedUnknownWithoutWorkStart(t *testing.T) {
	requestArrived := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	event := Event{StartedAt: requestArrived, TTFTMS: 500, InputTokens: 2000}
	DeriveTotals(&event)

	if event.PromptTokensPS != 0 {
		t.Fatalf("time to first token may include a load, got %.2f", event.PromptTokensPS)
	}
}

func TestDeriveTotalsKeepsReportedPromptSpeedOverEstimate(t *testing.T) {
	requestArrived := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	event := Event{
		StartedAt:      requestArrived,
		WorkStartedAt:  requestArrived,
		TTFTMS:         1000,
		InputTokens:    2000,
		PromptTokensPS: 3100,
	}
	DeriveTotals(&event)

	if event.PromptTokensPS != 3100 {
		t.Fatalf("reported prompt speed was overwritten, got %.2f", event.PromptTokensPS)
	}
}

func TestResponseObserverEstimatesPromptSpeedOnceWorkStartIsStamped(t *testing.T) {
	sink := &recordingEventSink{}
	stream := &pacedStream{
		delay: 20 * time.Millisecond,
		chunks: []string{
			`data: {"choices":[{"delta":{"content":"one"}}]}` + "\n\n",
			`data: {"choices":[],"usage":{"prompt_tokens":4000,"completion_tokens":1,"total_tokens":4001}}` + "\n\n",
			"data: [DONE]\n\n",
		},
	}
	requestArrived := time.Now().Add(-10 * time.Second)
	workStarted := time.Now()
	stampWorkStart := func(event *Event) { event.WorkStartedAt = workStarted }
	observer := NewResponseObserver(sink, Event{StartedAt: requestArrived}, "text/event-stream", stream, stampWorkStart)
	if _, err := io.ReadAll(observer); err != nil {
		t.Fatal(err)
	}

	recorded := sink.events[0]
	queueAndLoadSpeed := float64(recorded.InputTokens) / (float64(recorded.TTFTMS) / 1000)
	if recorded.PromptTokensPS <= queueAndLoadSpeed*10 {
		t.Fatalf("prompt speed must be measured from work start, got %.2f (from arrival %.2f)", recorded.PromptTokensPS, queueAndLoadSpeed)
	}
}

func TestStoreAveragesPromptSpeedInSummaryTimelineAndRecent(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	store.Record(Event{ModelID: "llm-a", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now, PromptTokensPS: 1000})
	store.Record(Event{ModelID: "llm-a", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now, PromptTokensPS: 3000})
	store.Record(Event{ModelID: "llm-a", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: now, FinishedAt: now})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	response, err := store.Query(context.Background(), Query{Period: PeriodAll, StartMS: now.Add(-time.Hour).UnixMilli(), EndMS: now.Add(time.Hour).UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if response.Summary.AveragePromptPS != 2000 {
		t.Fatalf("requests without a prompt speed must not dilute the average, got %.2f", response.Summary.AveragePromptPS)
	}
	if len(response.Timeline) != 1 || response.Timeline[0].AveragePromptPS != 2000 || response.Timeline[0].PromptPSSamples != 2 {
		t.Fatalf("unexpected timeline prompt speed %#v", response.Timeline)
	}
	recordedSpeeds := map[float64]bool{}
	for _, event := range response.Recent {
		recordedSpeeds[event.PromptTokensPS] = true
	}
	if !recordedSpeeds[1000] || !recordedSpeeds[3000] {
		t.Fatalf("recent events lost their prompt speed %#v", response.Recent)
	}
}

func TestRollupSummaryAveragesPromptSpeedPastRawRetention(t *testing.T) {
	store := newTestStore(t, "node-a")
	old := time.Now().UTC().Add(-60 * 24 * time.Hour)
	store.Record(Event{ModelID: "llm-a", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: old, FinishedAt: old, PromptTokensPS: 500})
	store.Record(Event{ModelID: "llm-a", Section: SectionLLM, StatusCode: 200, Success: true, StartedAt: old, FinishedAt: old, PromptTokensPS: 1500})
	if err := store.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}

	response, err := store.Query(context.Background(), Query{Period: PeriodAll, StartMS: old.Add(-time.Hour).UnixMilli(), EndMS: time.Now().UnixMilli()})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Recent) != 0 {
		t.Fatalf("old raw events should have been pruned %#v", response.Recent)
	}
	if response.Summary.AveragePromptPS != 1000 {
		t.Fatalf("rollup prompt speed average was lost, got %.2f", response.Summary.AveragePromptPS)
	}
}

func TestMergeWeighsTimelinePromptSpeedBySamples(t *testing.T) {
	merged := Merge(
		Response{Enabled: true, Timeline: []Timeline{{BucketStart: 1, AveragePromptPS: 1000, PromptPSSamples: 1}}},
		Response{Enabled: true, Timeline: []Timeline{{BucketStart: 1, AveragePromptPS: 4000, PromptPSSamples: 3}}},
	)

	if len(merged.Timeline) != 1 || merged.Timeline[0].AveragePromptPS != 3250 || merged.Timeline[0].PromptPSSamples != 4 {
		t.Fatalf("unexpected merged prompt speed %#v", merged.Timeline)
	}
}
