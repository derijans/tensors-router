package config

import (
	"fmt"

	"tensors-router/internal/flatyaml"
)

type MemoryFitConfig struct {
	DeepSpillPercent int
	TightHeadroomMB  int64
}

func defaultMemoryFitConfig() MemoryFitConfig {
	return MemoryFitConfig{DeepSpillPercent: 10, TightHeadroomMB: 512}
}

func memoryFitFields(memoryFit *MemoryFitConfig) flatyaml.Fields {
	return flatyaml.Fields{
		"deep_spill_percent": flatyaml.Int(&memoryFit.DeepSpillPercent),
		"tight_headroom_mb":  flatyaml.Int64(&memoryFit.TightHeadroomMB),
	}
}

func validateMemoryFit(cfg *Config) error {
	if cfg.MemoryFit.DeepSpillPercent < 1 || cfg.MemoryFit.DeepSpillPercent > 100 {
		return fmt.Errorf("memory_fit.deep_spill_percent must be between 1 and 100")
	}
	if cfg.MemoryFit.TightHeadroomMB < 0 {
		return fmt.Errorf("memory_fit.tight_headroom_mb cannot be negative")
	}
	return nil
}
