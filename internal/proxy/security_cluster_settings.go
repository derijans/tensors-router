package proxy

type SecurityClusterSettings struct {
	Role       string   `json:"role"`
	ServerBind string   `json:"server_bind"`
	Bind       string   `json:"bind"`
	PublicURL  string   `json:"public_url"`
	MasterURL  string   `json:"master_url"`
	SlaveURLs  []string `json:"slave_urls"`
}

func (settings SecurityClusterSettings) clone() SecurityClusterSettings {
	settings.SlaveURLs = append([]string{}, settings.SlaveURLs...)
	return settings
}
