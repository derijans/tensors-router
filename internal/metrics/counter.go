package metrics

import (
	"sort"
	"sync"
	"sync/atomic"
)

type CounterVec struct {
	name       string
	help       string
	labelNames []string
	maxSeries  int

	mu     sync.RWMutex
	series map[string]*counterSeries
}

type counterSeries struct {
	labels []Label
	value  atomic.Uint64
}

func (counter *CounterVec) Inc(labelValues ...string) {
	counter.Add(1, labelValues...)
}

func (counter *CounterVec) Add(delta uint64, labelValues ...string) {
	if counter == nil {
		return
	}
	mustLabelCount(counter.name, counter.labelNames, labelValues)
	counter.seriesFor(labelValues).value.Add(delta)
}

func (counter *CounterVec) seriesFor(labelValues []string) *counterSeries {
	key := seriesKey(labelValues)
	counter.mu.RLock()
	existing := counter.series[key]
	counter.mu.RUnlock()
	if existing != nil {
		return existing
	}
	counter.mu.Lock()
	defer counter.mu.Unlock()
	if existing = counter.series[key]; existing != nil {
		return existing
	}
	if len(counter.series) >= counter.maxSeries {
		labelValues = overflowValues(len(labelValues))
		key = seriesKey(labelValues)
		if existing = counter.series[key]; existing != nil {
			return existing
		}
	}
	created := &counterSeries{labels: pairLabels(counter.labelNames, append([]string(nil), labelValues...))}
	counter.series[key] = created
	return created
}

func (counter *CounterVec) samples() []sample {
	counter.mu.RLock()
	defer counter.mu.RUnlock()
	samples := make([]sample, 0, len(counter.series))
	for _, series := range counter.series {
		samples = append(samples, sample{name: counter.name, labels: series.labels, value: float64(series.value.Load())})
	}
	sort.Slice(samples, func(left, right int) bool { return labelsLess(samples[left].labels, samples[right].labels) })
	return samples
}
