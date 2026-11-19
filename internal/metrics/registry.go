package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
)

const ContentType = "text/plain; version=0.0.4; charset=utf-8"

type GaugeFamily struct {
	Name    string
	Help    string
	Samples []GaugeSample
}

type GaugeSample struct {
	Labels []Label
	Value  float64
}

type family interface {
	familyName() string
	familyHelp() string
	familyType() string
	samples() []sample
}

type Registry struct {
	mu       sync.Mutex
	families []family
	names    map[string]struct{}
}

func NewRegistry() *Registry {
	return &Registry{names: map[string]struct{}{}}
}

func (registry *Registry) NewCounter(name string, help string, labelNames ...string) *CounterVec {
	mustValidNames(name, labelNames)
	if !strings.HasSuffix(name, "_total") {
		panic(fmt.Sprintf("counter %q must end in _total", name))
	}
	counter := &CounterVec{name: name, help: help, labelNames: labelNames, maxSeries: DefaultMaxSeries, series: map[string]*counterSeries{}}
	registry.register(name, counter)
	return counter
}

func (registry *Registry) NewHistogram(name string, help string, buckets []float64, labelNames ...string) *HistogramVec {
	mustValidNames(name, labelNames)
	for _, labelName := range labelNames {
		if labelName == "le" {
			panic(fmt.Sprintf("histogram %q cannot use the reserved label le", name))
		}
	}
	sorted := append([]float64(nil), buckets...)
	sort.Float64s(sorted)
	histogram := &HistogramVec{name: name, help: help, labelNames: labelNames, buckets: sorted, maxSeries: DefaultMaxSeries, series: map[string]*histogramSeries{}}
	registry.register(name, histogram)
	return histogram
}

func (registry *Registry) register(name string, metric family) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, duplicate := registry.names[name]; duplicate {
		panic(fmt.Sprintf("metric %q registered twice", name))
	}
	registry.names[name] = struct{}{}
	registry.families = append(registry.families, metric)
}

func (registry *Registry) Write(w io.Writer, gauges []GaugeFamily) error {
	registry.mu.Lock()
	families := append([]family(nil), registry.families...)
	registry.mu.Unlock()
	for _, gauge := range gauges {
		if err := validGaugeFamily(gauge); err != nil {
			return err
		}
		families = append(families, gaugeFamily(gauge))
	}
	var text strings.Builder
	for _, metric := range families {
		writeFamily(&text, metric)
	}
	_, err := io.WriteString(w, text.String())
	return err
}
