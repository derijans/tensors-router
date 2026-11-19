package idleunload

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"time"
)

const (
	Key        = "router_idle_unload_seconds"
	MaxSeconds = 7 * 24 * 60 * 60
)

func Timeout(options map[string]json.RawMessage) (time.Duration, error) {
	raw, present := options[Key]
	if !present {
		return 0, nil
	}
	return Parse(raw)
}

func Parse(raw json.RawMessage) (time.Duration, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return 0, nil
	}
	var value float64
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return 0, fmt.Errorf("%s must be a whole number of seconds", Key)
	}
	if value != math.Trunc(value) {
		return 0, fmt.Errorf("%s must be a whole number of seconds", Key)
	}
	if value < 0 || value > MaxSeconds {
		return 0, fmt.Errorf("%s must be between 0 and %d", Key, MaxSeconds)
	}
	return time.Duration(value) * time.Second, nil
}
