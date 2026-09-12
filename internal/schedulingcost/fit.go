package schedulingcost

import "math"

type Sample struct {
	NodeID          string
	ModelID         string
	Section         string
	Arity           int
	Count           int64
	SumDuration     float64
	SumWork         [MaxWorkTerms]float64
	SumWorkDuration [MaxWorkTerms]float64
	SumWorkProduct  [MaxWorkTerms][MaxWorkTerms]float64
}

type LoadSample struct {
	NodeID         string
	ConfigFilename string
	Count          int64
	SumDuration    float64
}

type Estimate struct {
	BaseMS  float64
	SlopeMS [MaxWorkTerms]float64
	Arity   int
	Samples int64
}

func (estimate Estimate) PredictMS(work Work) (float64, bool) {
	if work.Arity() != estimate.Arity {
		return 0, false
	}
	total := estimate.BaseMS
	for index := 0; index < estimate.Arity; index++ {
		term := work.Term(index)
		if term < 0 {
			term = 0
		}
		total += estimate.SlopeMS[index] * term
	}
	return total, true
}

func (estimate Estimate) PredictQueueMS(count int64, totalWork Work) (float64, bool) {
	if count <= 0 {
		if totalWork.Arity() != 0 && totalWork.Arity() != estimate.Arity {
			return 0, false
		}
		return 0, true
	}
	if totalWork.Arity() != estimate.Arity {
		return 0, false
	}
	total := float64(count) * estimate.BaseMS
	for index := 0; index < estimate.Arity; index++ {
		term := totalWork.Term(index)
		if term < 0 {
			term = 0
		}
		total += estimate.SlopeMS[index] * term
	}
	return total, true
}

const minSamplesFloor = 2
const minRelativeWorkSpread = 0.05

func Fit(sample Sample, minSamples int64) (Estimate, bool) {
	if minSamples < minSamplesFloor {
		minSamples = minSamplesFloor
	}
	arity := sample.Arity
	if arity < 1 || arity > MaxWorkTerms {
		return Estimate{}, false
	}
	if sample.Count < minSamples {
		return Estimate{}, false
	}
	if !everyRegressorIsMeasurable(sample, arity) {
		return Estimate{}, false
	}

	moments, targets := normalEquationsFor(sample, arity)
	solution, ok := solveNormalEquations(moments, targets)
	if !ok {
		return Estimate{}, false
	}
	return estimateFromSolution(solution, arity, sample.Count)
}

func everyRegressorIsMeasurable(sample Sample, arity int) bool {
	count := float64(sample.Count)
	for index := 0; index < arity; index++ {
		denominator := count*sample.SumWorkProduct[index][index] - sample.SumWork[index]*sample.SumWork[index]
		if !relativeWorkSpreadIsMeasurable(denominator, sample.SumWork[index]) {
			return false
		}
	}
	return true
}

func normalEquationsFor(sample Sample, arity int) ([][]float64, []float64) {
	count := float64(sample.Count)
	size := arity + 1
	moments := make([][]float64, size)
	moments[0] = make([]float64, size)
	moments[0][0] = count
	for col := 0; col < arity; col++ {
		moments[0][col+1] = sample.SumWork[col]
	}
	for row := 0; row < arity; row++ {
		moments[row+1] = make([]float64, size)
		moments[row+1][0] = sample.SumWork[row]
		for col := 0; col < arity; col++ {
			moments[row+1][col+1] = sample.SumWorkProduct[row][col]
		}
	}
	targets := make([]float64, size)
	targets[0] = sample.SumDuration
	for row := 0; row < arity; row++ {
		targets[row+1] = sample.SumWorkDuration[row]
	}
	return moments, targets
}

func estimateFromSolution(solution []float64, arity int, sampleCount int64) (Estimate, bool) {
	base := solution[0]
	if math.IsNaN(base) || math.IsInf(base, 0) {
		return Estimate{}, false
	}
	var slopes [MaxWorkTerms]float64
	for index := 0; index < arity; index++ {
		slope := solution[index+1]
		if math.IsNaN(slope) || math.IsInf(slope, 0) || slope < 0 {
			return Estimate{}, false
		}
		slopes[index] = slope
	}
	if base < 0 {
		base = 0
	}
	return Estimate{BaseMS: base, SlopeMS: slopes, Arity: arity, Samples: sampleCount}, true
}

func relativeWorkSpreadIsMeasurable(denominator float64, sumWork float64) bool {
	if denominator <= 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		return false
	}
	if sumWork <= 0 {
		return false
	}
	threshold := minRelativeWorkSpread * sumWork
	return denominator >= threshold*threshold
}

func FitLoad(sample LoadSample, minSamples int64) (float64, bool) {
	if minSamples < 1 {
		minSamples = 1
	}
	if sample.Count < minSamples || sample.SumDuration <= 0 {
		return 0, false
	}
	mean := sample.SumDuration / float64(sample.Count)
	if math.IsNaN(mean) || math.IsInf(mean, 0) {
		return 0, false
	}
	return mean, true
}
