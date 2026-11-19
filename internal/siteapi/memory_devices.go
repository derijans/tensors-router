package siteapi

type NodeMemoryDevice struct {
	Vendor           string `json:"vendor"`
	Name             string `json:"name"`
	Source           string `json:"source"`
	DedicatedUsedMB  int64  `json:"dedicated_used_mb"`
	DedicatedTotalMB int64  `json:"dedicated_total_mb"`
	SharedUsedMB     int64  `json:"shared_used_mb,omitempty"`
	UsageKnown       bool   `json:"usage_known"`
	Unified          bool   `json:"unified,omitempty"`
}

type NodeSystemRAM struct {
	UsedMB  int64 `json:"used_mb"`
	TotalMB int64 `json:"total_mb"`
}
