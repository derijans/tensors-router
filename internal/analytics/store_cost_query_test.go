package analytics

import (
	"context"
	"math"
	"testing"
	"time"
)

func recordImageRequest(store *Store, now time.Time, modelID string, steps int64, width int64, height int64, count int64, durationMS int64, success bool) {
	store.Record(Event{
		ModelID:     modelID,
		Section:     SectionImage,
		BackendMode: "llama_sdcpp",
		Route:       "/sdapi/v1/*",
		EventType:   EventTypeRequest,
		StatusCode:  200,
		Success:     success,
		StartedAt:   now.Add(-time.Duration(durationMS) * time.Millisecond),
		FinishedAt:  now,
		DurationMS:  durationMS,
		ImageCount:  count,
		ImageWidth:  width,
		ImageHeight: height,
		ImageSteps:  steps,
	})
}

func TestCostSamplesAggregatesTheSumsAFitNeeds(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	type point struct {
		steps    int64
		width    int64
		height   int64
		count    int64
		duration int64
	}
	points := []point{
		{20, 512, 512, 1, 3000},
		{40, 512, 512, 1, 6000},
		{30, 1024, 1024, 1, 18000},
		{30, 512, 512, 2, 9000},
	}
	for _, value := range points {
		recordImageRequest(store, now, "sdxl", value.steps, value.width, value.height, value.count, value.duration, true)
	}

	samples, _, err := store.CostSamples(context.Background(), SectionImage, 24*time.Hour, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	sample := samples[0]
	if sample.NodeID != "node-a" || sample.ModelID != "sdxl" || sample.Section != SectionImage {
		t.Fatalf("unexpected sample identity %+v", sample)
	}

	var wantCount int64
	var wantWork, wantDuration, wantWorkDuration, wantWorkSquared float64
	for _, value := range points {
		count := value.count
		if count < 1 {
			count = 1
		}
		work := float64(value.steps) * float64(value.width) * float64(value.height) * float64(count)
		duration := float64(value.duration)
		wantCount++
		wantWork += work
		wantDuration += duration
		wantWorkDuration += work * duration
		wantWorkSquared += work * work
	}
	if sample.Count != wantCount {
		t.Fatalf("count = %d, want %d", sample.Count, wantCount)
	}
	for _, check := range []struct {
		name string
		got  float64
		want float64
	}{
		{"sum work", sample.SumWork, wantWork},
		{"sum duration", sample.SumDuration, wantDuration},
		{"sum work*duration", sample.SumWorkDuration, wantWorkDuration},
		{"sum work^2", sample.SumWorkSquared, wantWorkSquared},
	} {
		if math.Abs(check.got-check.want) > math.Abs(check.want)*1e-9 {
			t.Fatalf("%s = %v, want %v", check.name, check.got, check.want)
		}
	}
}

// Rows that predate the image_steps column, or requests that never named a step
// count, carry no usable work value. Including them would drag the fit toward a
// work of zero and inflate the fixed cost.
func TestCostSamplesExcludesRowsWithoutAWorkValue(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	recordImageRequest(store, now, "sdxl", 30, 1024, 1024, 1, 18000, true)
	recordImageRequest(store, now, "sdxl", 0, 1024, 1024, 1, 18000, true)
	recordImageRequest(store, now, "sdxl", 30, 0, 1024, 1, 18000, true)
	recordImageRequest(store, now, "sdxl", 30, 1024, 0, 1, 18000, true)

	samples, _, err := store.CostSamples(context.Background(), SectionImage, 24*time.Hour, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].Count != 1 {
		t.Fatalf("expected exactly the one row carrying a work value, got %+v", samples)
	}
}

func TestCostSamplesExcludesFailuresAndOtherSections(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	recordImageRequest(store, now, "sdxl", 30, 1024, 1024, 1, 18000, true)
	recordImageRequest(store, now, "sdxl", 30, 1024, 1024, 1, 18000, false)
	store.Record(Event{
		ModelID: "llm-a", Section: SectionLLM, EventType: EventTypeRequest, Success: true,
		StartedAt: now.Add(-time.Second), FinishedAt: now, DurationMS: 1000,
		ImageSteps: 30, ImageWidth: 1024, ImageHeight: 1024,
	})

	samples, _, err := store.CostSamples(context.Background(), SectionImage, 24*time.Hour, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].Count != 1 {
		t.Fatalf("failed or out-of-section rows leaked into the fit: %+v", samples)
	}
}

func TestCostSamplesHonoursTheWindow(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	recordImageRequest(store, now, "sdxl", 30, 1024, 1024, 1, 18000, true)
	recordImageRequest(store, now.Add(-48*time.Hour), "sdxl", 30, 1024, 1024, 1, 18000, true)

	samples, _, err := store.CostSamples(context.Background(), SectionImage, 24*time.Hour, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].Count != 1 {
		t.Fatalf("window was not applied: %+v", samples)
	}
}

func TestCostSamplesReportsModelLoadDurations(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	for _, durationMS := range []int64{18000, 20000} {
		store.Record(Event{
			ModelID:        "sdxl",
			Section:        SectionImage,
			EventType:      EventTypeModelLoad,
			Route:          "model_load",
			ConfigFilename: "sdxl.kcpps",
			Success:        true,
			StartedAt:      now.Add(-time.Duration(durationMS) * time.Millisecond),
			FinishedAt:     now,
			DurationMS:     durationMS,
		})
	}

	_, loads, err := store.CostSamples(context.Background(), SectionImage, 24*time.Hour, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(loads) != 1 {
		t.Fatalf("load samples = %d, want 1", len(loads))
	}
	if loads[0].ConfigFilename != "sdxl.kcpps" || loads[0].Count != 2 || loads[0].SumDuration != 38000 {
		t.Fatalf("unexpected load sample %+v", loads[0])
	}
}

// A day of large jobs squares to well past the range of exact integer
// arithmetic, so the work terms have to be accumulated as floating point.
func TestCostSamplesSurvivesLargeWorkValuesWithoutOverflow(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	for index := 0; index < 200; index++ {
		recordImageRequest(store, now, "sdxl", 50, 2048, 2048, 4, 60000, true)
	}

	samples, _, err := store.CostSamples(context.Background(), SectionImage, 24*time.Hour, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	work := 50.0 * 2048 * 2048 * 4
	wantSquared := 200 * work * work
	if got := samples[0].SumWorkSquared; math.Abs(got-wantSquared) > wantSquared*1e-9 {
		t.Fatalf("sum work^2 = %v, want %v", got, wantSquared)
	}
	if samples[0].SumWorkSquared <= 0 {
		t.Fatal("sum of squared work overflowed to a non-positive value")
	}
}

// The coefficients are fitted from the SQL expression and then applied to a value
// computed in Go. If the two ever diverge, predictions are made in different units
// from the model that produced them, and nothing else in the system would notice.
func TestImageWorkMatchesTheFitExpression(t *testing.T) {
	store := newTestStore(t, "node-a")
	now := time.Now().UTC()
	cases := []Event{
		{ImageSteps: 20, ImageWidth: 512, ImageHeight: 512, ImageCount: 1},
		{ImageSteps: 50, ImageWidth: 1024, ImageHeight: 768, ImageCount: 4},
		{ImageSteps: 30, ImageWidth: 2048, ImageHeight: 2048, ImageCount: 0},
		{ImageSteps: 1, ImageWidth: 64, ImageHeight: 64, ImageCount: 1},
	}
	var wantWork float64
	for index, event := range cases {
		wantWork += ImageWork(event)
		recordImageRequest(store, now, "sdxl", event.ImageSteps, event.ImageWidth, event.ImageHeight, event.ImageCount, int64(1000*(index+1)), true)
	}

	samples, _, err := store.CostSamples(context.Background(), SectionImage, 24*time.Hour, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("samples = %d, want 1", len(samples))
	}
	if math.Abs(samples[0].SumWork-wantWork) > wantWork*1e-12 {
		t.Fatalf("sql work total = %v, go work total = %v", samples[0].SumWork, wantWork)
	}
}

func TestImageWorkIsZeroForUnsizedRequests(t *testing.T) {
	for _, event := range []Event{
		{ImageWidth: 1024, ImageHeight: 1024, ImageCount: 1},
		{ImageSteps: 30, ImageHeight: 1024, ImageCount: 1},
		{ImageSteps: 30, ImageWidth: 1024, ImageCount: 1},
	} {
		if work := ImageWork(event); work != 0 {
			t.Fatalf("work = %v for %#v, want 0", work, event)
		}
	}
}

func TestImageWorkTreatsAnUnsetCountAsOne(t *testing.T) {
	single := ImageWork(Event{ImageSteps: 30, ImageWidth: 512, ImageHeight: 512, ImageCount: 1})
	unset := ImageWork(Event{ImageSteps: 30, ImageWidth: 512, ImageHeight: 512})
	if single != unset {
		t.Fatalf("unset count = %v, want the same as an explicit 1 (%v)", unset, single)
	}
}
