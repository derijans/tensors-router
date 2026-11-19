package siteapi

type MemoryFitBand struct {
	LowMB  float64 `json:"low_mb"`
	HighMB float64 `json:"high_mb"`
}

type ConfigMemoryFit struct {
	ModelID     string        `json:"model_id"`
	ConfigName  string        `json:"config_name"`
	Level       string        `json:"level"`
	Tier        string        `json:"tier"`
	VRAM        MemoryFitBand `json:"vram"`
	RAM         MemoryFitBand `json:"ram"`
	GPUFreeMB   float64       `json:"gpu_free_mb"`
	RAMFreeMB   float64       `json:"ram_free_mb"`
	SpillMB     float64       `json:"spill_mb,omitempty"`
	RAMOnly     bool          `json:"ram_only"`
	MultiGPU    bool          `json:"multi_gpu,omitempty"`
	Unavailable string        `json:"unavailable,omitempty"`
}

type NodeMemoryFit struct {
	NodeID  string            `json:"node_id"`
	Configs []ConfigMemoryFit `json:"configs"`
}
