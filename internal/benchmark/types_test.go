package benchmark

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMetricSerializesZeroCount(t *testing.T) {
	encoded, err := json.Marshal(countMetric("sections_failed", 0))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"value":0`) {
		t.Fatalf("zero count metric lost its value: %s", encoded)
	}
}
