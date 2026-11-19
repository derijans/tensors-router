package metrics

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const largestExactFloatInteger = 1 << 53

type sample struct {
	name   string
	labels []Label
	value  float64
}

type gaugeFamily GaugeFamily

func (gauge gaugeFamily) familyName() string { return gauge.Name }
func (gauge gaugeFamily) familyHelp() string { return gauge.Help }
func (gauge gaugeFamily) familyType() string { return "gauge" }
func (gauge gaugeFamily) samples() []sample {
	samples := make([]sample, len(gauge.Samples))
	for index, gaugeSample := range gauge.Samples {
		samples[index] = sample{name: gauge.Name, labels: gaugeSample.Labels, value: gaugeSample.Value}
	}
	return samples
}

func (counter *CounterVec) familyName() string { return counter.name }
func (counter *CounterVec) familyHelp() string { return counter.help }
func (counter *CounterVec) familyType() string { return "counter" }

func (histogram *HistogramVec) familyName() string { return histogram.name }
func (histogram *HistogramVec) familyHelp() string { return histogram.help }
func (histogram *HistogramVec) familyType() string { return "histogram" }

func validGaugeFamily(gauge GaugeFamily) error {
	if !metricNamePattern.MatchString(gauge.Name) {
		return fmt.Errorf("invalid gauge name %q", gauge.Name)
	}
	for _, gaugeSample := range gauge.Samples {
		for _, label := range gaugeSample.Labels {
			if !labelNamePattern.MatchString(label.Name) {
				return fmt.Errorf("invalid label name %q on gauge %q", label.Name, gauge.Name)
			}
		}
	}
	return nil
}

func writeFamily(w *strings.Builder, metric family) {
	samples := metric.samples()
	if len(samples) == 0 {
		return
	}
	fmt.Fprintf(w, "# HELP %s %s\n", metric.familyName(), escapeHelp(metric.familyHelp()))
	fmt.Fprintf(w, "# TYPE %s %s\n", metric.familyName(), metric.familyType())
	for _, entry := range samples {
		w.WriteString(entry.name)
		writeLabels(w, entry.labels)
		w.WriteByte(' ')
		w.WriteString(formatValue(entry.value))
		w.WriteByte('\n')
	}
}

func writeLabels(w *strings.Builder, labels []Label) {
	if len(labels) == 0 {
		return
	}
	w.WriteByte('{')
	for index, label := range labels {
		if index > 0 {
			w.WriteByte(',')
		}
		w.WriteString(label.Name)
		w.WriteString(`="`)
		w.WriteString(escapeLabelValue(label.Value))
		w.WriteByte('"')
	}
	w.WriteByte('}')
}

var (
	labelValueEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	helpEscaper       = strings.NewReplacer(`\`, `\\`, "\n", `\n`)
)

func escapeLabelValue(value string) string { return labelValueEscaper.Replace(value) }
func escapeHelp(value string) string       { return helpEscaper.Replace(value) }

func formatValue(value float64) string {
	switch {
	case math.IsInf(value, 1):
		return "+Inf"
	case math.IsInf(value, -1):
		return "-Inf"
	case math.IsNaN(value):
		return "NaN"
	case value == math.Trunc(value) && math.Abs(value) < largestExactFloatInteger:
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return strconv.FormatFloat(value, 'g', -1, 64)
	}
}

func labelsLess(left []Label, right []Label) bool {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index].Value != right[index].Value {
			return left[index].Value < right[index].Value
		}
	}
	return len(left) < len(right)
}
