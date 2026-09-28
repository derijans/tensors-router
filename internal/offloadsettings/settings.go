package offloadsettings

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type Settings struct {
	RefreshInterval     time.Duration
	SampleWindow        time.Duration
	MinSamples          int
	BackendDepth        int
	GrantTTL            time.Duration
	ContextReserve      int
	RestoreDelay        time.Duration
	ProbeIdle           time.Duration
	FasterHelperSlots   int
	SlowerHelperSlots   int
	ProbeHelperSlots    int
	HoldForFasterHelper bool
	DecisionRetention   time.Duration
}

type Values map[string]string

func Defaults() Settings {
	return Resolve(nil, nil).Settings
}

func (settings Settings) Values() Values {
	values := make(Values, len(fields))
	for _, field := range fields {
		values[field.Key] = field.read(settings)
	}
	return values
}

func (settings Settings) Fingerprint() string {
	digest := sha256.New()
	for _, field := range fields {
		digest.Write([]byte(field.Key + "=" + field.read(settings) + "\n"))
	}
	return hex.EncodeToString(digest.Sum(nil))[:12]
}

func Parse(values Values) (Settings, error) {
	settings := Defaults()
	for key, value := range values {
		field, err := fieldFor(key)
		if err != nil {
			return Settings{}, err
		}
		normalized, err := field.normalize(value)
		if err != nil {
			return Settings{}, err
		}
		field.assign(&settings, normalized)
	}
	return settings, nil
}
