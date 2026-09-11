package schedulingcost

import "math"

// ProfileKey identifies which (node, model)'s measured token profile a lookup
// wants, mirroring ModelKey without a section: a token profile only ever
// applies to the text lane.
type ProfileKey struct {
	NodeID  string
	ModelID string
}

// TokenProfileSample is the measured relationship between what a client sends
// and what the backend reports, reduced to the sums a ratio and its spread
// need. Ratio is per-row (promptBytes/inputTokens for one request), not a
// pooled sum of bytes over sum of tokens, because a per-row ratio is what
// EstimatePromptTokens applies to a single future request.
type TokenProfileSample struct {
	NodeID           string
	ModelID          string
	Count            int64
	SumRatio         float64
	SumRatioSquared  float64
	SumOutput        float64
	SumOutputSquared float64
}

// TokenProfile is the fitted, ready-to-use form: a bytes-per-token ratio
// already pulled conservative by the model's own measured spread, and a
// reserved-output floor already pulled up the same way.
type TokenProfile struct {
	ConservativeBytesPerToken float64
	ReservedOutputTokens      float64
	Samples                   int64
}

// conservativeMarginK is how many standard deviations the estimate is pulled
// toward the safe side: down for the bytes-per-token ratio (fewer bytes per
// token means more estimated tokens), up for reserved output. It is a constant
// rather than a config key because the margin this package produces is already
// the model's own measured spread — there is nothing left for an operator to
// tune without re-introducing the guess this design exists to avoid.
const conservativeMarginK = 2.0

// minBytesPerToken and maxBytesPerToken bound a plausible ratio. Outside this
// range the two columns are not measuring the same requests — most often a
// multimodal request whose body is dominated by an embedded image, which would
// drag a text-only ratio in the unsafe direction if it were allowed through.
const minBytesPerToken = 0.5
const maxBytesPerToken = 64.0

// FitTokenProfile rejects on thin history, on a non-positive sum, or when the
// mean ratio falls outside a plausible range. A model with no profile is
// unqualified: no cost ordering and no context gate, never a guess.
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
	if math.IsNaN(meanRatio) || math.IsInf(meanRatio, 0) {
		return TokenProfile{}, false
	}
	if meanRatio < minBytesPerToken || meanRatio > maxBytesPerToken {
		return TokenProfile{}, false
	}
	ratioStdDev := stdDev(count, sample.SumRatio, sample.SumRatioSquared)
	conservativeRatio := meanRatio - conservativeMarginK*ratioStdDev
	if conservativeRatio < minBytesPerToken {
		conservativeRatio = minBytesPerToken
	}

	meanOutput := sample.SumOutput / count
	outputStdDev := stdDev(count, sample.SumOutput, sample.SumOutputSquared)
	reservedOutput := meanOutput + conservativeMarginK*outputStdDev
	if math.IsNaN(reservedOutput) || math.IsInf(reservedOutput, 0) || reservedOutput < 0 {
		reservedOutput = meanOutput
	}

	return TokenProfile{
		ConservativeBytesPerToken: conservativeRatio,
		ReservedOutputTokens:      reservedOutput,
		Samples:                   sample.Count,
	}, true
}

// stdDev computes a population standard deviation from raw sums, clamping a
// negative variance (possible only from floating-point cancellation on a
// near-zero-spread sample) to zero rather than propagating a NaN.
func stdDev(count float64, sum float64, sumSquared float64) float64 {
	mean := sum / count
	variance := sumSquared/count - mean*mean
	if variance < 0 {
		variance = 0
	}
	return math.Sqrt(variance)
}

// EstimatePromptTokens converts a request's raw body size into a token count
// using the conservative ratio: a lower bytes-per-token ratio yields more
// estimated tokens, which is the safe direction for a capacity gate. It
// reports false for a zero-value profile — an unmeasured model is never gated
// on a guess.
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

// RequiredContext is the full pre-dispatch context-window requirement: the
// estimated prompt plus however much output must be reserved. Reserved output
// is the largest of what the client asked for, what this model's own traffic
// measures as typical, and the configured floor — a request whose client
// stated no limit still reserves at least the model's own history, not the
// bare floor.
func (profile TokenProfile) RequiredContext(promptBytes int64, clientMaxTokens int, reserveFloor int) (int, bool) {
	promptTokens, ok := profile.EstimatePromptTokens(promptBytes)
	if !ok {
		return 0, false
	}
	reserved := profile.ReservedOutputTokens
	if float64(clientMaxTokens) > reserved {
		reserved = float64(clientMaxTokens)
	}
	if float64(reserveFloor) > reserved {
		reserved = float64(reserveFloor)
	}
	return promptTokens + int(math.Ceil(reserved)), true
}
