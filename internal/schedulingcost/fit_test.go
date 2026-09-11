package schedulingcost

import (
	"math"
	"testing"
)

func sampleFromPoints(points [][2]float64) Sample {
	sample := Sample{NodeID: "node-a", ModelID: "sdxl", Section: "image", Arity: 1}
	for _, point := range points {
		work, duration := point[0], point[1]
		sample.Count++
		sample.SumWork[0] += work
		sample.SumDuration += duration
		sample.SumWorkDuration[0] += work * duration
		sample.SumWorkProduct[0][0] += work * work
	}
	return sample
}

func linearPoints(base float64, slope float64, works []float64) [][2]float64 {
	points := make([][2]float64, 0, len(works))
	for _, work := range works {
		points = append(points, [2]float64{work, base + slope*work})
	}
	return points
}

func imageWorks(count int) []float64 {
	works := make([]float64, 0, count)
	for index := 0; index < count; index++ {
		steps := float64(10 + index%40)
		pixels := float64(512*512) * float64(1+index%4)
		works = append(works, steps*pixels)
	}
	return works
}

func TestFitRecoversKnownCoefficients(t *testing.T) {
	const base = 1400
	const slope = 0.0000123
	sample := sampleFromPoints(linearPoints(base, slope, imageWorks(60)))

	estimate, ok := Fit(sample, 20)
	if !ok {
		t.Fatal("fit rejected a clean linear relationship")
	}
	if math.Abs(estimate.BaseMS-base) > 1e-4 {
		t.Fatalf("base = %v, want %v", estimate.BaseMS, base)
	}
	if math.Abs(estimate.SlopeMS[0]-slope) > 1e-12 {
		t.Fatalf("slope = %v, want %v", estimate.SlopeMS[0], slope)
	}
	if estimate.Samples != sample.Count {
		t.Fatalf("samples = %d, want %d", estimate.Samples, sample.Count)
	}
	if estimate.Arity != 1 {
		t.Fatalf("arity = %d, want 1", estimate.Arity)
	}
}

// TestArityOneFitMatchesTheClosedForm is what makes "image is the degenerate
// case of the general solver" a verified claim rather than a hope: the general
// normal-equation solve, on a single-regressor sample, must reproduce the
// closed-form OLS coefficients bit-for-bit within floating point tolerance.
func TestArityOneFitMatchesTheClosedForm(t *testing.T) {
	sample := sampleFromPoints(linearPoints(1234.5, 0.0000789, imageWorks(50)))
	estimate, ok := Fit(sample, 20)
	if !ok {
		t.Fatal("fit rejected a clean linear relationship")
	}

	count := float64(sample.Count)
	denominator := count*sample.SumWorkProduct[0][0] - sample.SumWork[0]*sample.SumWork[0]
	wantSlope := (count*sample.SumWorkDuration[0] - sample.SumWork[0]*sample.SumDuration) / denominator
	wantBase := (sample.SumDuration - wantSlope*sample.SumWork[0]) / count

	if math.Abs(estimate.SlopeMS[0]-wantSlope) > 1e-9 {
		t.Fatalf("slope = %v, want closed form %v", estimate.SlopeMS[0], wantSlope)
	}
	if math.Abs(estimate.BaseMS-wantBase) > 1e-6 {
		t.Fatalf("base = %v, want closed form %v", estimate.BaseMS, wantBase)
	}
}

func TestFitPredictsUnseenWork(t *testing.T) {
	const base = 900
	const slope = 0.00002
	sample := sampleFromPoints(linearPoints(base, slope, imageWorks(40)))

	estimate, ok := Fit(sample, 20)
	if !ok {
		t.Fatal("fit rejected a clean linear relationship")
	}
	unseen := 30.0 * 1024 * 1024
	want := base + slope*unseen
	got, ok := estimate.PredictMS(ImageWork(unseen))
	if !ok {
		t.Fatal("prediction rejected for matching arity")
	}
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("prediction = %v, want %v", got, want)
	}
}

func TestFitRejectsTooFewSamples(t *testing.T) {
	sample := sampleFromPoints(linearPoints(500, 0.001, imageWorks(19)))
	if _, ok := Fit(sample, 20); ok {
		t.Fatal("fit accepted 19 samples with a floor of 20")
	}
}

// A node that only ever served one resolution at one step count carries no
// information about how duration scales, so it must stay unqualified rather than
// report a slope derived from rounding error.
func TestFitRejectsDegenerateWorkVariance(t *testing.T) {
	identical := make([]float64, 40)
	for index := range identical {
		identical[index] = 30 * 1024 * 1024
	}
	points := linearPoints(700, 0.00001, identical)
	for index := range points {
		points[index][1] += float64(index%5) * 12
	}
	if _, ok := Fit(sampleFromPoints(points), 20); ok {
		t.Fatal("fit accepted samples with no work variance")
	}
}

func TestFitRejectsNegativeSlope(t *testing.T) {
	sample := sampleFromPoints(linearPoints(9000, -0.00002, imageWorks(40)))
	if _, ok := Fit(sample, 20); ok {
		t.Fatal("fit accepted a negative slope")
	}
}

func TestFitClampsNegativeInterceptToZero(t *testing.T) {
	points := linearPoints(-500, 0.0001, imageWorks(40))
	estimate, ok := Fit(sampleFromPoints(points), 20)
	if !ok {
		t.Fatal("fit rejected a usable slope because the intercept was negative")
	}
	if estimate.BaseMS != 0 {
		t.Fatalf("base = %v, want 0", estimate.BaseMS)
	}
}

func TestFitLoadRejectsEmptyHistory(t *testing.T) {
	if _, ok := FitLoad(LoadSample{NodeID: "node-a", ConfigFilename: "sdxl.kcpps"}, 1); ok {
		t.Fatal("load fit accepted zero samples")
	}
}

func TestFitLoadAveragesRecordedLoads(t *testing.T) {
	mean, ok := FitLoad(LoadSample{NodeID: "node-a", ConfigFilename: "sdxl.kcpps", Count: 4, SumDuration: 76000}, 1)
	if !ok {
		t.Fatal("load fit rejected usable samples")
	}
	if mean != 19000 {
		t.Fatalf("mean = %v, want 19000", mean)
	}
}

// Work values clustered within a fraction of a percent of each other produce a
// slope dominated by measurement noise, so they must be rejected just as firmly
// as identical ones. Without the relative spread test this fit is accepted and
// the slope extrapolates wildly.
func TestFitRejectsNarrowWorkSpread(t *testing.T) {
	works := make([]float64, 40)
	for index := range works {
		works[index] = 30*1024*1024 + float64(index%5)*100
	}
	points := linearPoints(700, 0.00001, works)
	for index := range points {
		points[index][1] += float64(index%7) * 9
	}
	sample := sampleFromPoints(points)

	count := float64(sample.Count)
	denominator := count*sample.SumWorkProduct[0][0] - sample.SumWork[0]*sample.SumWork[0]
	if denominator <= 0 {
		t.Fatal("test data is exactly degenerate, so it would not exercise the spread test")
	}

	if _, ok := Fit(sample, 20); ok {
		t.Fatal("fit accepted work values spread far below the relative floor")
	}
}

func TestFitAcceptsRealisticImageWorkSpread(t *testing.T) {
	sample := sampleFromPoints(linearPoints(1200, 0.00002, imageWorks(50)))
	if _, ok := Fit(sample, 20); !ok {
		t.Fatal("fit rejected a realistic spread of image resolutions and step counts")
	}
}

// textSampleFromPoints builds a two-term sample from (prefill, decode,
// duration) triples, mirroring sampleFromPoints for the text lane.
func textSampleFromPoints(points [][3]float64) Sample {
	sample := Sample{NodeID: "node-a", ModelID: "llama", Section: "llm", Arity: 2}
	for _, point := range points {
		prefill, decode, duration := point[0], point[1], point[2]
		sample.Count++
		sample.SumDuration += duration
		sample.SumWork[0] += prefill
		sample.SumWork[1] += decode
		sample.SumWorkDuration[0] += prefill * duration
		sample.SumWorkDuration[1] += decode * duration
		sample.SumWorkProduct[0][0] += prefill * prefill
		sample.SumWorkProduct[1][1] += decode * decode
		sample.SumWorkProduct[0][1] += prefill * decode
		sample.SumWorkProduct[1][0] += prefill * decode
	}
	return sample
}

// textPointsIndependent generates synthetic (prefill, decode, duration) rows
// from known coefficients with prefill and decode varied independently of one
// another, which is what a real two-term fit needs to separate the slopes.
func textPointsIndependent(base, prefillSlope, decodeSlope float64, n int) [][3]float64 {
	points := make([][3]float64, 0, n)
	for index := 0; index < n; index++ {
		prefill := float64(50 + (index*37)%900)
		decode := float64(20 + (index*53)%400)
		duration := base + prefillSlope*prefill + decodeSlope*decode
		points = append(points, [3]float64{prefill, decode, duration})
	}
	return points
}

func TestTwoTermFitSeparatesPrefillFromDecode(t *testing.T) {
	const base = 80.0
	const prefillSlope = 0.9
	const decodeSlope = 25.0
	sample := textSampleFromPoints(textPointsIndependent(base, prefillSlope, decodeSlope, 80))

	estimate, ok := Fit(sample, 20)
	if !ok {
		t.Fatal("fit rejected a clean two-term linear relationship")
	}
	if estimate.Arity != 2 {
		t.Fatalf("arity = %d, want 2", estimate.Arity)
	}
	if math.Abs(estimate.BaseMS-base) > 1e-3 {
		t.Fatalf("base = %v, want %v", estimate.BaseMS, base)
	}
	if math.Abs(estimate.SlopeMS[0]-prefillSlope) > 1e-6 {
		t.Fatalf("prefill slope = %v, want %v", estimate.SlopeMS[0], prefillSlope)
	}
	if math.Abs(estimate.SlopeMS[1]-decodeSlope) > 1e-6 {
		t.Fatalf("decode slope = %v, want %v", estimate.SlopeMS[1], decodeSlope)
	}
}

func TestTwoTermFitRejectsDegenerateSpreadInEitherRegressor(t *testing.T) {
	points := textPointsIndependent(80, 0.9, 25.0, 60)
	for index := range points {
		points[index][0] = 400
	}
	if _, ok := Fit(textSampleFromPoints(points), 20); ok {
		t.Fatal("fit accepted a constant prefill regressor")
	}

	points = textPointsIndependent(80, 0.9, 25.0, 60)
	for index := range points {
		points[index][1] = 150
	}
	if _, ok := Fit(textSampleFromPoints(points), 20); ok {
		t.Fatal("fit accepted a constant decode regressor")
	}
}

func TestTwoTermFitRejectsCollinearRegressors(t *testing.T) {
	points := make([][3]float64, 0, 60)
	for index := 0; index < 60; index++ {
		prefill := float64(50 + (index*37)%900)
		decode := prefill * 0.5
		duration := 80 + 0.9*prefill + 25.0*decode
		points = append(points, [3]float64{prefill, decode, duration})
	}
	if _, ok := Fit(textSampleFromPoints(points), 20); ok {
		t.Fatal("fit accepted perfectly collinear regressors")
	}
}

func TestTwoTermFitRejectsNegativeSlope(t *testing.T) {
	sample := textSampleFromPoints(textPointsIndependent(80, -0.5, 25.0, 60))
	if _, ok := Fit(sample, 20); ok {
		t.Fatal("fit accepted a negative prefill slope")
	}
}

func TestFitRejectsThinHistoryAtArityTwo(t *testing.T) {
	sample := textSampleFromPoints(textPointsIndependent(80, 0.9, 25.0, 19))
	if _, ok := Fit(sample, 20); ok {
		t.Fatal("fit accepted 19 two-term samples with a floor of 20")
	}
}
