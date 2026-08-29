package schedulingcost

import "math"

// Sample is the aggregated form of every request analytics recorded for one
// (node, model, section), reduced to the five sums ordinary least squares needs.
type Sample struct {
	NodeID          string
	ModelID         string
	Section         string
	Count           int64
	SumWork         float64
	SumDuration     float64
	SumWorkDuration float64
	SumWorkSquared  float64
}

// LoadSample is the same reduction for model load events, which have no work
// term and collapse to a mean.
type LoadSample struct {
	NodeID         string
	ConfigFilename string
	Count          int64
	SumDuration    float64
}

type Estimate struct {
	BaseMS  float64
	SlopeMS float64
	Samples int64
}

func (estimate Estimate) PredictMS(work float64) float64 {
	if work < 0 {
		work = 0
	}
	return estimate.BaseMS + estimate.SlopeMS*work
}

// Fit rejects rather than guesses. A node whose history is too thin, whose work
// values do not vary, or whose fitted slope is negative stays unqualified and is
// never predictively scheduled.
func Fit(sample Sample, minSamples int64) (Estimate, bool) {
	if minSamples < 2 {
		minSamples = 2
	}
	if sample.Count < minSamples {
		return Estimate{}, false
	}
	count := float64(sample.Count)
	denominator := count*sample.SumWorkSquared - sample.SumWork*sample.SumWork
	if !workSpreadIsUsable(denominator, sample.SumWork) {
		return Estimate{}, false
	}
	slope := (count*sample.SumWorkDuration - sample.SumWork*sample.SumDuration) / denominator
	if math.IsNaN(slope) || math.IsInf(slope, 0) || slope < 0 {
		return Estimate{}, false
	}
	base := (sample.SumDuration - slope*sample.SumWork) / count
	if math.IsNaN(base) || math.IsInf(base, 0) {
		return Estimate{}, false
	}
	if base < 0 {
		base = 0
	}
	return Estimate{BaseMS: base, SlopeMS: slope, Samples: sample.Count}, true
}

// workSpreadIsUsable requires the observed work values to vary by a meaningful
// fraction of their own mean. A node that only ever served one resolution at one
// step count carries no information about how duration scales with work, so a
// slope derived from that history would be noise dressed as a measurement.
// Expressed as a relative standard deviation, the test is
// sqrt(denominator)/sumWork >= minRelativeWorkSpread.
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
