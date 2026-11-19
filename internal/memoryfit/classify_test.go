package memoryfit

import "testing"

func measured(vramLow float64, vramHigh float64, ramLow float64, ramHigh float64) Estimate {
	return Estimate{Tier: TierMeasured, VRAM: Band{LowMB: vramLow, HighMB: vramHigh}, RAM: Band{LowMB: ramLow, HighMB: ramHigh}}
}

func TestClassifyGPUBackendLevels(t *testing.T) {
	thresholds := DefaultThresholds()
	capacity := Capacity{GPUFreeMB: 10000, RAMFreeMB: 20000}
	for name, test := range map[string]struct {
		estimate Estimate
		want     Level
	}{
		"fits with room":                 {measured(6000, 7000, 100, 200), LevelGreen},
		"under 512 MB of VRAM to spare":  {measured(9000, 9600, 100, 200), LevelYellow},
		"under 512 MB of RAM to spare":   {measured(1000, 2000, 19000, 19700), LevelYellow},
		"spills a little":                {measured(9500, 10500, 0, 0), LevelLightAmber},
		"spills more than a tenth":       {measured(11000, 12000, 0, 0), LevelDeepAmber},
		"cannot fit VRAM and RAM":        {measured(25000, 26000, 6000, 6000), LevelRed},
		"high end over, low end fits":    {measured(20000, 40000, 0, 0), LevelDeepAmber},
		"exactly at combined capacity":   {measured(30000, 30000, 0, 0), LevelDeepAmber},
		"low end one MB above capacity":  {measured(30001, 30001, 0, 0), LevelRed},
		"nothing learned yet":            {Estimate{Tier: TierUnknown}, LevelUnknown},
		"spill threshold boundary exact": {measured(10000, 11000, 0, 0), LevelLightAmber},
	} {
		if got := Classify(test.estimate, capacity, thresholds).Level; got != test.want {
			t.Errorf("%s: level=%s, want %s", name, got, test.want)
		}
	}
}

func TestClassifyNeverReportsRedWhenTheLowEndFits(t *testing.T) {
	capacity := Capacity{GPUFreeMB: 8000, RAMFreeMB: 8000}
	for high := 16000.0; high <= 1e6; high *= 2 {
		if Classify(measured(15999, high, 0, high), capacity, DefaultThresholds()).Level == LevelRed {
			t.Fatalf("red with low end inside capacity and high=%v", high)
		}
	}
}

func TestClassifyRAMOnlyBackendUsesThreeLevels(t *testing.T) {
	capacity := Capacity{RAMFreeMB: 16000, RAMOnly: true}
	for name, test := range map[string]struct {
		estimate Estimate
		want     Level
	}{
		"fits":       {measured(0, 0, 8000, 9000), LevelGreen},
		"tight":      {measured(0, 0, 15000, 15800), LevelYellow},
		"cannot fit": {measured(0, 0, 16001, 17000), LevelRed},
		"over GPU-sized VRAM share but RAM only never turns amber": {measured(4000, 5000, 9000, 10000), LevelGreen},
	} {
		if got := Classify(test.estimate, capacity, DefaultThresholds()).Level; got != test.want {
			t.Errorf("%s: level=%s, want %s", name, got, test.want)
		}
	}
}
