package offloadsettings

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Kind string

const (
	KindDuration Kind = "duration"
	KindInteger  Kind = "integer"
)

type Field struct {
	Key         string
	Kind        Kind
	Description string
	Default     string
	normalize   func(value string) (string, error)
	assign      func(settings *Settings, normalized string)
	read        func(settings Settings) string
}

const maximumSlotsOrDepth = 64

var fields = []Field{
	durationField("scheduling_refresh_interval", "How often every node refits its costs and the master replans lending.", "1m0s",
		func(settings *Settings) *time.Duration { return &settings.RefreshInterval }),
	durationField("scheduling_sample_window", "How far back finished requests count toward cost fits.", "24h0m0s",
		func(settings *Settings) *time.Duration { return &settings.SampleWindow }),
	integerField("scheduling_min_samples", "Finished requests a model needs in the sample window before its cost is priced. Until then lending only probes.", 20, 2, 100000,
		func(settings *Settings) *int { return &settings.MinSamples }),
	integerField("scheduling_backend_depth", "Requests per lane admitted to a backend at once; the rest wait in the router where they can be lent.", 2, 1, maximumSlotsOrDepth,
		func(settings *Settings) *int { return &settings.BackendDepth }),
	durationField("scheduling_grant_ttl", "How long a lending lease lives unless the master renews it.", "30s",
		func(settings *Settings) *time.Duration { return &settings.GrantTTL }),
	integerField("scheduling_context_reserve", "Tokens kept free on top of a text request's predicted context when choosing a helper.", 256, 0, 1<<20,
		func(settings *Settings) *int { return &settings.ContextReserve }),
	durationField("offload_restore_delay", "Idle time before a helper reloads the model it had before borrowing.", "1.5s",
		func(settings *Settings) *time.Duration { return &settings.RestoreDelay }),
	durationField("offload_probe_idle", "Idle time after which an unpriced helper is probed with one borrowed request.", "5s",
		func(settings *Settings) *time.Duration { return &settings.ProbeIdle }),
	integerField("offload_faster_helper_slots", "Requests lent at once to a helper predicted to be at least as fast as the owner.", 2, 1, maximumSlotsOrDepth,
		func(settings *Settings) *int { return &settings.FasterHelperSlots }),
	integerField("offload_slower_helper_slots", "Requests lent at once to a helper that pays off but is slower than the owner.", 1, 1, maximumSlotsOrDepth,
		func(settings *Settings) *int { return &settings.SlowerHelperSlots }),
	integerField("offload_probe_helper_slots", "Requests lent at once while probing an unpriced pair.", 1, 1, maximumSlotsOrDepth,
		func(settings *Settings) *int { return &settings.ProbeHelperSlots }),
	durationField("offload_decisions_retention", "How long lending decisions are kept in the log.", "720h0m0s",
		func(settings *Settings) *time.Duration { return &settings.DecisionRetention }),
}

func Fields() []Field {
	return append([]Field(nil), fields...)
}

func Known(key string) bool {
	_, err := fieldFor(key)
	return err == nil
}

func Normalize(key string, value string) (string, error) {
	field, err := fieldFor(key)
	if err != nil {
		return "", err
	}
	return field.normalize(value)
}

func fieldFor(key string) (Field, error) {
	for _, field := range fields {
		if field.Key == key {
			return field, nil
		}
	}
	return Field{}, fmt.Errorf("unknown lending setting %q", key)
}

func durationField(key string, description string, fallback string, target func(*Settings) *time.Duration) Field {
	return Field{
		Key:         key,
		Kind:        KindDuration,
		Description: description,
		Default:     fallback,
		normalize: func(value string) (string, error) {
			parsed, err := time.ParseDuration(strings.TrimSpace(value))
			if err != nil {
				return "", fmt.Errorf("%s: %w", key, err)
			}
			if parsed <= 0 {
				return "", fmt.Errorf("%s must be positive", key)
			}
			return parsed.String(), nil
		},
		assign: func(settings *Settings, normalized string) {
			parsed, _ := time.ParseDuration(normalized)
			*target(settings) = parsed
		},
		read: func(settings Settings) string { return target(&settings).String() },
	}
}

func integerField(key string, description string, fallback int, minimum int, maximum int, target func(*Settings) *int) Field {
	return Field{
		Key:         key,
		Kind:        KindInteger,
		Description: description,
		Default:     strconv.Itoa(fallback),
		normalize: func(value string) (string, error) {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return "", fmt.Errorf("%s must be a whole number", key)
			}
			if parsed < minimum || parsed > maximum {
				return "", fmt.Errorf("%s must be between %d and %d", key, minimum, maximum)
			}
			return strconv.Itoa(parsed), nil
		},
		assign: func(settings *Settings, normalized string) {
			parsed, _ := strconv.Atoi(normalized)
			*target(settings) = parsed
		},
		read: func(settings Settings) string { return strconv.Itoa(*target(&settings)) },
	}
}
