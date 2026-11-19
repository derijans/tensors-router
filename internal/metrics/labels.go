package metrics

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	DefaultMaxSeries = 1000
	OverflowLabel    = "other"
)

type Label struct {
	Name  string
	Value string
}

var (
	metricNamePattern = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelNamePattern  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

func mustValidNames(name string, labelNames []string) {
	if !metricNamePattern.MatchString(name) {
		panic(fmt.Sprintf("invalid metric name %q", name))
	}
	for _, labelName := range labelNames {
		if !labelNamePattern.MatchString(labelName) || strings.HasPrefix(labelName, "__") {
			panic(fmt.Sprintf("invalid label name %q on metric %q", labelName, name))
		}
	}
}

func seriesKey(values []string) string {
	return strings.Join(values, "\xff")
}

func overflowValues(count int) []string {
	values := make([]string, count)
	for index := range values {
		values[index] = OverflowLabel
	}
	return values
}

func pairLabels(names []string, values []string) []Label {
	labels := make([]Label, len(names))
	for index, name := range names {
		labels[index] = Label{Name: name, Value: values[index]}
	}
	return labels
}

func mustLabelCount(name string, names []string, values []string) {
	if len(names) != len(values) {
		panic(fmt.Sprintf("metric %q takes %d label values, got %d", name, len(names), len(values)))
	}
}
