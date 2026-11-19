package metrics

import (
	"math"
	"sort"
	"strconv"
	"sync"
)

var DurationBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600}

type HistogramVec struct {
	name       string
	help       string
	labelNames []string
	buckets    []float64
	maxSeries  int

	mu     sync.Mutex
	series map[string]*histogramSeries
}

type histogramSeries struct {
	labels       []Label
	bucketCounts []uint64
	count        uint64
	sum          float64
}

func (histogram *HistogramVec) Observe(value float64, labelValues ...string) {
	if histogram == nil || math.IsNaN(value) {
		return
	}
	mustLabelCount(histogram.name, histogram.labelNames, labelValues)
	histogram.mu.Lock()
	defer histogram.mu.Unlock()
	series := histogram.seriesForLocked(labelValues)
	for index, upperBound := range histogram.buckets {
		if value <= upperBound {
			series.bucketCounts[index]++
		}
	}
	series.count++
	series.sum += value
}

func (histogram *HistogramVec) seriesForLocked(labelValues []string) *histogramSeries {
	key := seriesKey(labelValues)
	if existing := histogram.series[key]; existing != nil {
		return existing
	}
	if len(histogram.series) >= histogram.maxSeries {
		labelValues = overflowValues(len(labelValues))
		key = seriesKey(labelValues)
		if existing := histogram.series[key]; existing != nil {
			return existing
		}
	}
	created := &histogramSeries{
		labels:       pairLabels(histogram.labelNames, append([]string(nil), labelValues...)),
		bucketCounts: make([]uint64, len(histogram.buckets)),
	}
	histogram.series[key] = created
	return created
}

func (histogram *HistogramVec) samples() []sample {
	histogram.mu.Lock()
	ordered := make([]*histogramSeries, 0, len(histogram.series))
	for _, series := range histogram.series {
		ordered = append(ordered, series)
	}
	sort.Slice(ordered, func(left, right int) bool { return labelsLess(ordered[left].labels, ordered[right].labels) })
	samples := make([]sample, 0, len(ordered)*(len(histogram.buckets)+3))
	for _, series := range ordered {
		samples = append(samples, histogram.seriesSamplesLocked(series)...)
	}
	histogram.mu.Unlock()
	return samples
}

func (histogram *HistogramVec) seriesSamplesLocked(series *histogramSeries) []sample {
	samples := make([]sample, 0, len(histogram.buckets)+3)
	for index, upperBound := range histogram.buckets {
		samples = append(samples, sample{
			name:   histogram.name + "_bucket",
			labels: withLabel(series.labels, "le", strconv.FormatFloat(upperBound, 'g', -1, 64)),
			value:  float64(series.bucketCounts[index]),
		})
	}
	samples = append(samples,
		sample{name: histogram.name + "_bucket", labels: withLabel(series.labels, "le", "+Inf"), value: float64(series.count)},
		sample{name: histogram.name + "_sum", labels: series.labels, value: series.sum},
		sample{name: histogram.name + "_count", labels: series.labels, value: float64(series.count)},
	)
	return samples
}

func withLabel(labels []Label, name string, value string) []Label {
	extended := make([]Label, 0, len(labels)+1)
	extended = append(extended, labels...)
	return append(extended, Label{Name: name, Value: value})
}
