package offloadsettings

import (
	"testing"
	"time"
)

func entryFor(t *testing.T, resolution Resolution, key string) Entry {
	t.Helper()
	for _, entry := range resolution.Entries {
		if entry.Key == key {
			return entry
		}
	}
	t.Fatalf("no entry for %q", key)
	return Entry{}
}

func TestResolveLayersDefaultsThenConfigThenDatabase(t *testing.T) {
	resolution := Resolve(
		Values{"offload_probe_idle": "10s", "scheduling_min_samples": "8"},
		Values{"offload_probe_idle": "2s"},
	)

	probe := entryFor(t, resolution, "offload_probe_idle")
	if probe.Source != SourceDatabase || probe.Effective != "2s" || probe.Config != "10s" || probe.Default != "5s" {
		t.Fatalf("probe idle entry = %+v, want the database value to win over config and default", probe)
	}
	if samples := entryFor(t, resolution, "scheduling_min_samples"); samples.Source != SourceConfig || samples.Effective != "8" {
		t.Fatalf("min samples entry = %+v, want the config value to win over the default", samples)
	}
	if ttl := entryFor(t, resolution, "scheduling_grant_ttl"); ttl.Source != SourceDefault || ttl.Effective != "30s" {
		t.Fatalf("grant ttl entry = %+v, want the default", ttl)
	}
	if resolution.Settings.ProbeIdle != 2*time.Second || resolution.Settings.MinSamples != 8 || resolution.Settings.GrantTTL != 30*time.Second {
		t.Fatalf("settings = %+v, want each field from its winning layer", resolution.Settings)
	}
}

func TestResolveShowsButDoesNotApplyAStoredValueThatNoLongerValidates(t *testing.T) {
	resolution := Resolve(Values{"scheduling_backend_depth": "3"}, Values{"scheduling_backend_depth": "0"})

	depth := entryFor(t, resolution, "scheduling_backend_depth")
	if depth.Source != SourceConfig || depth.Effective != "3" || depth.Override != "0" {
		t.Fatalf("backend depth entry = %+v, want the invalid override shown and the config value applied", depth)
	}
}

func TestNormalizeRejectsValuesOutsideTheirRange(t *testing.T) {
	for key, value := range map[string]string{
		"scheduling_min_samples":         "1",
		"scheduling_backend_depth":       "0",
		"scheduling_context_reserve":     "-1",
		"offload_probe_idle":             "0s",
		"scheduling_grant_ttl":           "soon",
		"offload_probe_helper_slots":     "65",
		"offload_hold_for_faster_helper": "sometimes",
		"not_a_setting":                  "1",
	} {
		if _, err := Normalize(key, value); err == nil {
			t.Errorf("Normalize(%q, %q) accepted", key, value)
		}
	}
}

func TestNormalizeWritesDurationsCanonically(t *testing.T) {
	if normalized, err := Normalize("offload_restore_delay", " 1500ms "); err != nil || normalized != "1.5s" {
		t.Fatalf("Normalize = %q, %v, want 1.5s", normalized, err)
	}
}

func TestHoldForFasterHelperDefaultsOnAndTurnsOffFromAnyLayer(t *testing.T) {
	if !Defaults().HoldForFasterHelper {
		t.Fatal("holding for a faster helper is off by default, want on")
	}
	if normalized, err := Normalize("offload_hold_for_faster_helper", " FALSE "); err != nil || normalized != "false" {
		t.Fatalf("Normalize = %q, %v, want false", normalized, err)
	}
	if Resolve(Values{"offload_hold_for_faster_helper": "false"}, nil).Settings.HoldForFasterHelper {
		t.Fatal("config file value false did not switch holding off")
	}
	if !Resolve(Values{"offload_hold_for_faster_helper": "false"}, Values{"offload_hold_for_faster_helper": "true"}).Settings.HoldForFasterHelper {
		t.Fatal("database value true did not override the config file")
	}
}

func TestParsedValuesRoundTripAndKeepTheirFingerprint(t *testing.T) {
	original := Resolve(nil, Values{"offload_faster_helper_slots": "4", "scheduling_refresh_interval": "2s"}).Settings

	parsed, err := Parse(original.Values())
	if err != nil {
		t.Fatal(err)
	}
	if parsed != original || parsed.Fingerprint() != original.Fingerprint() {
		t.Fatalf("parsed %+v, want %+v", parsed, original)
	}
	if Defaults().Fingerprint() == original.Fingerprint() {
		t.Fatal("different settings share a fingerprint")
	}
}
