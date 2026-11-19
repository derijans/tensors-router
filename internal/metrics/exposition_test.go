package metrics

import (
	"strings"
	"testing"
)

func render(t *testing.T, registry *Registry, gauges ...GaugeFamily) string {
	t.Helper()
	var out strings.Builder
	if err := registry.Write(&out, gauges); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestWriteProducesTheTextExpositionFormat(t *testing.T) {
	registry := NewRegistry()
	requests := registry.NewCounter("app_requests_total", "Requests served.", "route", "status")
	requests.Inc("chat", "200")
	requests.Add(2, "chat", "200")
	requests.Inc("embed", "500")
	durations := registry.NewHistogram("app_duration_seconds", "Request duration.", []float64{1, 0.5}, "route")
	durations.Observe(0.3, "chat")
	durations.Observe(0.7, "chat")

	got := render(t, registry, GaugeFamily{Name: "app_users", Help: "Active users.", Samples: []GaugeSample{{Labels: []Label{{Name: "runtime", Value: "text"}}, Value: 2}}})
	want := `# HELP app_requests_total Requests served.
# TYPE app_requests_total counter
app_requests_total{route="chat",status="200"} 3
app_requests_total{route="embed",status="500"} 1
# HELP app_duration_seconds Request duration.
# TYPE app_duration_seconds histogram
app_duration_seconds_bucket{route="chat",le="0.5"} 1
app_duration_seconds_bucket{route="chat",le="1"} 2
app_duration_seconds_bucket{route="chat",le="+Inf"} 2
app_duration_seconds_sum{route="chat"} 1
app_duration_seconds_count{route="chat"} 2
# HELP app_users Active users.
# TYPE app_users gauge
app_users{runtime="text"} 2
`
	if got != want {
		t.Fatalf("exposition mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestWriteEscapesHostileLabelValues(t *testing.T) {
	registry := NewRegistry()
	registry.NewCounter("app_errors_total", "Errors.", "model").Inc("a\"b\\c\nd} 1\nfake_metric 9")

	got := render(t, registry)
	if !strings.Contains(got, `app_errors_total{model="a\"b\\c\nd} 1\nfake_metric 9"} 1`) {
		t.Fatalf("label value was not escaped:\n%s", got)
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(line, "fake_metric") {
			t.Fatalf("label value injected a sample line:\n%s", got)
		}
	}
}

func TestCounterFoldsSeriesBeyondTheCapIntoOther(t *testing.T) {
	registry := NewRegistry()
	counter := registry.NewCounter("app_models_total", "Models.", "model")
	counter.maxSeries = 2
	for _, model := range []string{"a", "b", "c", "d"} {
		counter.Inc(model)
	}

	got := render(t, registry)
	if strings.Count(got, "app_models_total{") != 3 {
		t.Fatalf("series were not capped:\n%s", got)
	}
	if !strings.Contains(got, `app_models_total{model="other"} 2`) {
		t.Fatalf("overflow series missing:\n%s", got)
	}
}

func TestHistogramBucketsAreCumulativeAndEndInCount(t *testing.T) {
	registry := NewRegistry()
	histogram := registry.NewHistogram("app_load_seconds", "Loads.", DurationBuckets)
	for _, value := range []float64{0.01, 3, 3, 700} {
		histogram.Observe(value)
	}

	var previous float64
	for _, entry := range histogram.samples() {
		if entry.name != "app_load_seconds_bucket" {
			continue
		}
		if entry.value < previous {
			t.Fatalf("bucket counts decrease at %v", entry.labels)
		}
		previous = entry.value
	}
	if previous != 4 {
		t.Fatalf("+Inf bucket=%v, want the observation count 4", previous)
	}
}

func TestWriteRejectsAnInvalidGaugeName(t *testing.T) {
	if err := NewRegistry().Write(&strings.Builder{}, []GaugeFamily{{Name: "bad name"}}); err == nil {
		t.Fatal("an invalid gauge name was written")
	}
}

func TestNilVectorsAreSafeToUse(t *testing.T) {
	var counter *CounterVec
	var histogram *HistogramVec
	counter.Inc("x")
	histogram.Observe(1, "x")
}
