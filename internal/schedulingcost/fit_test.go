package schedulingcost

import (
	"math"
	"testing"
)

func sampleFromPoints(points [][2]float64) Sample {
	sample := Sample{NodeID: "node-a", ModelID: "sdxl", Section: "image"}
	for _, point := range points {
		work, duration := point[0], point[1]
		sample.Count++
		sample.SumWork += work
		sample.SumDuration += duration
		sample.SumWorkDuration += work * duration
		sample.SumWorkSquared += work * work
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
	if math.Abs(estimate.SlopeMS-slope) > 1e-12 {
		t.Fatalf("slope = %v, want %v", estimate.SlopeMS, slope)
	}
	if estimate.Samples != sample.Count {
		t.Fatalf("samples = %d, want %d", estimate.Samples, sample.Count)
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
	if got := estimate.PredictMS(unseen); math.Abs(got-want) > 1e-6 {
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
	denominator := count*sample.SumWorkSquared - sample.SumWork*sample.SumWork
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
