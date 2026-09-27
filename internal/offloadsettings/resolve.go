package offloadsettings

type Source string

const (
	SourceDefault  Source = "default"
	SourceConfig   Source = "config"
	SourceDatabase Source = "db"
)

type Entry struct {
	Key         string `json:"key"`
	Kind        Kind   `json:"kind"`
	Description string `json:"description"`
	Default     string `json:"default"`
	Config      string `json:"config,omitempty"`
	Override    string `json:"override,omitempty"`
	Effective   string `json:"effective"`
	Source      Source `json:"source"`
}

type Resolution struct {
	Settings Settings
	Entries  []Entry
}

// Resolve layers the settings from lowest to highest priority: built-in
// defaults, then the config file, then values stored in the database. A stored
// value that no longer validates is shown but not applied.
func Resolve(fileValues Values, overrides Values) Resolution {
	var resolution Resolution
	for _, field := range fields {
		entry := Entry{Key: field.Key, Kind: field.Kind, Description: field.Description, Default: field.Default, Effective: field.Default, Source: SourceDefault}
		if value, present := fileValues[field.Key]; present {
			entry.Config = value
			if normalized, err := field.normalize(value); err == nil {
				entry.Config, entry.Effective, entry.Source = normalized, normalized, SourceConfig
			}
		}
		if value, present := overrides[field.Key]; present {
			entry.Override = value
			if normalized, err := field.normalize(value); err == nil {
				entry.Override, entry.Effective, entry.Source = normalized, normalized, SourceDatabase
			}
		}
		field.assign(&resolution.Settings, entry.Effective)
		resolution.Entries = append(resolution.Entries, entry)
	}
	return resolution
}
