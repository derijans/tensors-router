package schedulingcost

import "math"

// Sample is the aggregated form of every request analytics recorded for one
// (node, model, section), reduced to the moment sums an ordinary least squares
// fit needs for however many regressors this lane's work has (Arity). Image
// fits one term (pixel-steps); text fits two (prefill and decode tokens). Only
// SumWork[:Arity], SumWorkDuration[:Arity] and SumWorkProduct[:Arity][:Arity]
// are meaningful; the rest sit at zero and are ignored.
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

// LoadSample is the same reduction for model load events, which have no work
// term and collapse to a mean.
type LoadSample struct {
	NodeID         string
	ConfigFilename string
	Count          int64
	SumDuration    float64
}

// Estimate is a fitted duration = BaseMS + sum(SlopeMS[i] * work_i) model.
// Arity says how many of SlopeMS's entries are meaningful.
type Estimate struct {
	BaseMS  float64
	SlopeMS [MaxWorkTerms]float64
	Arity   int
	Samples int64
}

// PredictMS prices one request of the given size. It reports false when work's
// arity does not match the estimate's — pricing a text hint against an image
// fit, or the reverse, is a unit error the caller must never be allowed to make
// silently.
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

// PredictQueueMS prices a whole pending queue rather than one request: each
// entry pays the fixed per-request cost once, and the variable cost scales with
// the summed work.
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

// Fit rejects rather than guesses. A node whose history is too thin, whose work
// values do not vary in some regressor, whose regressors are too collinear to
// separate, or whose fitted slope is negative in any regressor stays
// unqualified and is never predictively scheduled.
func Fit(sample Sample, minSamples int64) (Estimate, bool) {
	if minSamples < 2 {
		minSamples = 2
	}
	arity := sample.Arity
	if arity < 1 || arity > MaxWorkTerms {
		return Estimate{}, false
	}
	if sample.Count < minSamples {
		return Estimate{}, false
	}
	count := float64(sample.Count)

	for index := 0; index < arity; index++ {
		denominator := count*sample.SumWorkProduct[index][index] - sample.SumWork[index]*sample.SumWork[index]
		if !workSpreadIsUsable(denominator, sample.SumWork[index]) {
			return Estimate{}, false
		}
	}

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

	solution, ok := solveNormalEquations(moments, targets)
	if !ok {
		return Estimate{}, false
	}
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
	return Estimate{BaseMS: base, SlopeMS: slopes, Arity: arity, Samples: sample.Count}, true
}

// workSpreadIsUsable requires the observed work values to vary by a meaningful
// fraction of their own mean. A node that only ever served one resolution at one
// step count (or, for text, one prompt length) carries no information about how
// duration scales with that regressor, so a slope derived from that history
// would be noise dressed as a measurement. Expressed as a relative standard
// deviation, the test is sqrt(denominator)/sumWork >= minRelativeWorkSpread.
func workSpreadIsUsable(denominator float64, sumWork float64) bool {
	if denominator <= 0 || math.IsNaN(denominator) || math.IsInf(denominator, 0) {
		return false
	}
	if sumWork <= 0 {
		return false
	}
	threshold := minRelativeWorkSpread * sumWork
	return denominator >= threshold*threshold
}

const minRelativeWorkSpread = 0.05

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
