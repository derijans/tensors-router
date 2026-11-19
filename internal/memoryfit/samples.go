package memoryfit

import "math"

func vramOf(sample Sample) float64 { return float64(sample.VRAMMB) }
func ramOf(sample Sample) float64  { return float64(sample.RAMMB) }

func bandOf(samples []Sample, value func(Sample) float64) Band {
	band := Band{LowMB: math.Inf(1), HighMB: math.Inf(-1)}
	for _, sample := range samples {
		band.LowMB = math.Min(band.LowMB, value(sample))
		band.HighMB = math.Max(band.HighMB, value(sample))
	}
	if math.IsInf(band.LowMB, 1) {
		return Band{}
	}
	return band
}

func nearestContext(samples []Sample, target int) int {
	nearest := samples[0].ContextSize
	for _, sample := range samples[1:] {
		if absInt(sample.ContextSize-target) < absInt(nearest-target) {
			nearest = sample.ContextSize
		}
	}
	return nearest
}

func samplesAt(samples []Sample, contextSize int) []Sample {
	var matching []Sample
	for _, sample := range samples {
		if sample.ContextSize == contextSize {
			matching = append(matching, sample)
		}
	}
	return matching
}

func fittedAcrossContexts(samples []Sample, target int, value func(Sample) float64) (Band, bool) {
	contexts := map[int]struct{}{}
	var meanX, meanY float64
	for _, sample := range samples {
		contexts[sample.ContextSize] = struct{}{}
		meanX += float64(sample.ContextSize)
		meanY += value(sample)
	}
	if len(contexts) < 2 {
		return Band{}, false
	}
	count := float64(len(samples))
	meanX, meanY = meanX/count, meanY/count
	var covariance, variance float64
	for _, sample := range samples {
		dx := float64(sample.ContextSize) - meanX
		covariance += dx * (value(sample) - meanY)
		variance += dx * dx
	}
	slope := covariance / variance
	intercept := meanY - slope*meanX
	var worstResidual float64
	for _, sample := range samples {
		worstResidual = math.Max(worstResidual, math.Abs(value(sample)-(intercept+slope*float64(sample.ContextSize))))
	}
	predicted := intercept + slope*float64(target)
	return widened(Band{LowMB: predicted - worstResidual, HighMB: predicted + worstResidual}, contextScaledMargin), true
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
