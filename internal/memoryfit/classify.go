package memoryfit

type Level string

const (
	LevelUnknown    Level = "unknown"
	LevelGreen      Level = "green"
	LevelYellow     Level = "yellow"
	LevelLightAmber Level = "light_amber"
	LevelDeepAmber  Level = "deep_amber"
	LevelRed        Level = "red"
)

type Capacity struct {
	GPUFreeMB float64 `json:"gpu_free_mb"`
	RAMFreeMB float64 `json:"ram_free_mb"`
	RAMOnly   bool    `json:"ram_only"`
}

type Thresholds struct {
	DeepSpillFraction float64
	TightHeadroomMB   float64
}

func DefaultThresholds() Thresholds {
	return Thresholds{DeepSpillFraction: 0.10, TightHeadroomMB: 512}
}

type Verdict struct {
	Level   Level   `json:"level"`
	SpillMB float64 `json:"spill_mb,omitempty"`
}

func Classify(estimate Estimate, capacity Capacity, thresholds Thresholds) Verdict {
	if estimate.Tier == TierUnknown {
		return Verdict{Level: LevelUnknown}
	}
	if capacity.RAMOnly {
		return classifyAgainstRAM(estimate, capacity, thresholds)
	}
	if estimate.VRAM.LowMB+estimate.RAM.LowMB > capacity.GPUFreeMB+capacity.RAMFreeMB {
		return Verdict{Level: LevelRed}
	}
	if spill := estimate.VRAM.HighMB - capacity.GPUFreeMB; spill > 0 {
		if spill > thresholds.DeepSpillFraction*estimate.VRAM.HighMB {
			return Verdict{Level: LevelDeepAmber, SpillMB: spill}
		}
		return Verdict{Level: LevelLightAmber, SpillMB: spill}
	}
	gpuHeadroom := capacity.GPUFreeMB - estimate.VRAM.HighMB
	ramHeadroom := capacity.RAMFreeMB - estimate.RAM.HighMB
	if gpuHeadroom < thresholds.TightHeadroomMB || ramHeadroom < thresholds.TightHeadroomMB {
		return Verdict{Level: LevelYellow}
	}
	return Verdict{Level: LevelGreen}
}

func classifyAgainstRAM(estimate Estimate, capacity Capacity, thresholds Thresholds) Verdict {
	needLow := estimate.VRAM.LowMB + estimate.RAM.LowMB
	needHigh := estimate.VRAM.HighMB + estimate.RAM.HighMB
	switch {
	case needLow > capacity.RAMFreeMB:
		return Verdict{Level: LevelRed}
	case capacity.RAMFreeMB-needHigh < thresholds.TightHeadroomMB:
		return Verdict{Level: LevelYellow}
	default:
		return Verdict{Level: LevelGreen}
	}
}
