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

func Resolve(configFileValues Values, databaseOverrides Values) Resolution {
	var resolution Resolution
	for _, field := range fields {
		entry := Entry{Key: field.Key, Kind: field.Kind, Description: field.Description, Default: field.Default, Effective: field.Default, Source: SourceDefault}
		if value, present := configFileValues[field.Key]; present {
			entry.Config = value
			if normalized, err := field.normalize(value); err == nil {
				entry.Config, entry.Effective, entry.Source = normalized, normalized, SourceConfig
			}
		}
		if value, present := databaseOverrides[field.Key]; present {
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
