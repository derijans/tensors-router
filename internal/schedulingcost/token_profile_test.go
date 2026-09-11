package schedulingcost

import "testing"

func ratioSample(count int64, ratios []float64, outputs []float64) TokenProfileSample {
	sample := TokenProfileSample{NodeID: "node-a", ModelID: "llama", Count: count}
	for _, ratio := range ratios {
		sample.SumRatio += ratio
		sample.SumRatioSquared += ratio * ratio
	}
	for _, output := range outputs {
		sample.SumOutput += output
		sample.SumOutputSquared += output * output
	}
	return sample
}

func TestTokenProfileRejectsUnmeasuredModel(t *testing.T) {
	if _, ok := FitTokenProfile(TokenProfileSample{NodeID: "node-a", ModelID: "llama"}, 5); ok {
		t.Fatal("fit accepted a model with zero samples")
	}
	var zero TokenProfile
	if _, ok := zero.EstimatePromptTokens(1000); ok {
		t.Fatal("zero-value profile produced an estimate")
	}
	if _, ok := zero.RequiredContext(1000, 0, 256); ok {
		t.Fatal("zero-value profile produced a required context")
	}
}

func TestTokenProfileRejectsImplausibleRatio(t *testing.T) {
	ratios := make([]float64, 20)
	for index := range ratios {
		ratios[index] = 200 + float64(index%3)
	}
	outputs := make([]float64, 20)
	for index := range outputs {
		outputs[index] = 100 + float64(index%5)
	}
	sample := ratioSample(20, ratios, outputs)
	if _, ok := FitTokenProfile(sample, 5); ok {
		t.Fatal("fit accepted a ratio of ~200 bytes/token")
	}
}

// TestConservativeRatioWidensWithMeasuredVariance pins that the safety margin
// is the model's own measured spread, not a configured percent: two profiles
// with the same mean ratio but different spread must produce different,
// correctly-ordered estimates.
func TestConservativeRatioWidensWithMeasuredVariance(t *testing.T) {
	stableRatios := make([]float64, 30)
	stableOutputs := make([]float64, 30)
	for index := range stableRatios {
		stableRatios[index] = 4.0
		stableOutputs[index] = 200
	}
	stable := ratioSample(30, stableRatios, stableOutputs)

	noisyRatios := make([]float64, 30)
	noisyOutputs := make([]float64, 30)
	for index := range noisyRatios {
		if index%2 == 0 {
			noisyRatios[index] = 3.0
			noisyOutputs[index] = 100
		} else {
			noisyRatios[index] = 5.0
			noisyOutputs[index] = 300
		}
	}
	noisy := ratioSample(30, noisyRatios, noisyOutputs)

	stableProfile, ok := FitTokenProfile(stable, 5)
	if !ok {
		t.Fatal("fit rejected a stable ratio")
	}
	noisyProfile, ok := FitTokenProfile(noisy, 5)
	if !ok {
		t.Fatal("fit rejected a noisy ratio")
	}

	if noisyProfile.ConservativeBytesPerToken >= stableProfile.ConservativeBytesPerToken {
		t.Fatalf("noisy ratio %v was not pulled more conservative than stable ratio %v",
			noisyProfile.ConservativeBytesPerToken, stableProfile.ConservativeBytesPerToken)
	}

	stableTokens, _ := stableProfile.EstimatePromptTokens(4000)
	noisyTokens, _ := noisyProfile.EstimatePromptTokens(4000)
	if noisyTokens <= stableTokens {
		t.Fatalf("noisy profile estimated %d tokens, want more than stable profile's %d", noisyTokens, stableTokens)
	}
}

func TestRequiredContextHonoursTheClientsMaxTokens(t *testing.T) {
	ratios := make([]float64, 30)
	outputs := make([]float64, 30)
	for index := range ratios {
		ratios[index] = 4.0
		outputs[index] = 100
	}
	profile, ok := FitTokenProfile(ratioSample(30, ratios, outputs), 5)
	if !ok {
		t.Fatal("fit rejected a stable profile")
	}
	required, ok := profile.RequiredContext(4000, 5000, 0)
	if !ok {
		t.Fatal("required context rejected for a qualified profile")
	}
	promptTokens, _ := profile.EstimatePromptTokens(4000)
	if required != promptTokens+5000 {
		t.Fatalf("required context = %d, want %d", required, promptTokens+5000)
	}
}

func TestRequiredContextFallsBackToTheMeasuredOutput(t *testing.T) {
	ratios := make([]float64, 30)
	outputs := make([]float64, 30)
	for index := range ratios {
		ratios[index] = 4.0
		outputs[index] = 300
	}
	profile, ok := FitTokenProfile(ratioSample(30, ratios, outputs), 5)
	if !ok {
		t.Fatal("fit rejected a stable profile")
	}
	required, ok := profile.RequiredContext(4000, 0, 0)
	if !ok {
		t.Fatal("required context rejected for a qualified profile")
	}
	promptTokens, _ := profile.EstimatePromptTokens(4000)
	if required <= promptTokens {
		t.Fatalf("required context %d did not reserve any measured output over prompt tokens %d", required, promptTokens)
	}
}

func TestRequiredContextRespectsTheReserveFloor(t *testing.T) {
	ratios := make([]float64, 30)
	outputs := make([]float64, 30)
	for index := range ratios {
		ratios[index] = 4.0
		outputs[index] = 10
	}
	profile, ok := FitTokenProfile(ratioSample(30, ratios, outputs), 5)
	if !ok {
		t.Fatal("fit rejected a stable profile")
	}
	required, ok := profile.RequiredContext(4000, 0, 5000)
	if !ok {
		t.Fatal("required context rejected for a qualified profile")
	}
	promptTokens, _ := profile.EstimatePromptTokens(4000)
	if required != promptTokens+5000 {
		t.Fatalf("required context = %d, want %d (floor applied)", required, promptTokens+5000)
	}
}
