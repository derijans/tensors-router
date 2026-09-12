package schedulingcost

import "math"

type ProfileKey struct {
	NodeID  string
	ModelID string
}

type TokenProfileSample struct {
	NodeID           string
	ModelID          string
	Count            int64
	SumRatio         float64
	SumRatioSquared  float64
	SumOutput        float64
	SumOutputSquared float64
}

type TokenProfile struct {
	ConservativeBytesPerToken float64
	ReservedOutputTokens      float64
	Samples                   int64
}

const safetyMarginStdDevs = 2.0
const minPlausibleBytesPerToken = 0.5
const maxPlausibleBytesPerToken = 64.0

func FitTokenProfile(sample TokenProfileSample, minSamples int64) (TokenProfile, bool) {
	if minSamples < 1 {
		minSamples = 1
	}
	if sample.Count < minSamples {
		return TokenProfile{}, false
	}
	if sample.SumRatio <= 0 || sample.SumOutput <= 0 {
		return TokenProfile{}, false
	}
	count := float64(sample.Count)
	meanRatio := sample.SumRatio / count
	if !ratioIsPlausible(meanRatio) {
		return TokenProfile{}, false
	}

	return TokenProfile{
		ConservativeBytesPerToken: conservativeBytesPerToken(sample, count, meanRatio),
		ReservedOutputTokens:      reservedOutputTokens(sample, count),
		Samples:                   sample.Count,
	}, true
}

func ratioIsPlausible(meanRatio float64) bool {
	if math.IsNaN(meanRatio) || math.IsInf(meanRatio, 0) {
		return false
	}
	return meanRatio >= minPlausibleBytesPerToken && meanRatio <= maxPlausibleBytesPerToken
}

func conservativeBytesPerToken(sample TokenProfileSample, count float64, meanRatio float64) float64 {
	ratioStdDev := populationStdDev(count, sample.SumRatio, sample.SumRatioSquared)
	conservativeRatio := meanRatio - safetyMarginStdDevs*ratioStdDev
	if conservativeRatio < minPlausibleBytesPerToken {
		conservativeRatio = minPlausibleBytesPerToken
	}
	return conservativeRatio
}

func reservedOutputTokens(sample TokenProfileSample, count float64) float64 {
	meanOutput := sample.SumOutput / count
	outputStdDev := populationStdDev(count, sample.SumOutput, sample.SumOutputSquared)
	reserved := meanOutput + safetyMarginStdDevs*outputStdDev
	if math.IsNaN(reserved) || math.IsInf(reserved, 0) || reserved < 0 {
		return meanOutput
	}
	return reserved
}

func populationStdDev(count float64, sum float64, sumSquared float64) float64 {
	mean := sum / count
	variance := sumSquared/count - mean*mean
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}

func (profile TokenProfile) EstimatePromptTokens(promptBytes int64) (int, bool) {
	if profile.ConservativeBytesPerToken <= 0 || promptBytes <= 0 {
		return 0, false
	}
	tokens := float64(promptBytes) / profile.ConservativeBytesPerToken
	if math.IsNaN(tokens) || math.IsInf(tokens, 0) {
		return 0, false
	}
	return int(math.Ceil(tokens)), true
}

func (profile TokenProfile) RequiredContext(promptBytes int64, clientMaxTokens int, reserveFloor int) (int, bool) {
	promptTokens, ok := profile.EstimatePromptTokens(promptBytes)
	if !ok {
		return 0, false
	}
	reserved := largestReserve(profile.ReservedOutputTokens, float64(clientMaxTokens), float64(reserveFloor))
	return promptTokens + int(math.Ceil(reserved)), true
}

func largestReserve(measured float64, clientRequested float64, floor float64) float64 {
	largest := measured
	if clientRequested > largest {
		largest = clientRequested
	}
	if floor > largest {
		largest = floor
	}
	return largest
}
