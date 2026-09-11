package schedulingcost

import "encoding/json"

// MaxWorkTerms is the highest arity any lane fits today: the text lane's
// prefill and decode terms. A lane with fewer terms simply leaves the rest at
// zero; Arity is what marks how many are meaningful.
const MaxWorkTerms = 2

// Work is one request's size expressed in the regressors its lane is fitted on.
// The image lane states one term (pixel-steps); the text lane states two
// (prefill and decode tokens) because the same model runs them at different
// rates. A zero-arity Work is the "this request was never sized" case: it never
// qualifies for prediction, matching an unbuffered or unsized request.
type Work struct {
	terms [MaxWorkTerms]float64
	arity int
}

// ImageWork is the image lane's single-term work vector.
func ImageWork(pixelSteps float64) Work {
	if pixelSteps <= 0 {
		return Work{}
	}
	return Work{terms: [MaxWorkTerms]float64{pixelSteps}, arity: 1}
}

// TextWork is the text lane's two-term work vector: prefill (input tokens) and
// decode (output tokens) priced separately because they run at different rates
// on the same model.
func TextWork(inputTokens float64, outputTokens float64) Work {
	if inputTokens <= 0 && outputTokens <= 0 {
		return Work{}
	}
	if inputTokens < 0 {
		inputTokens = 0
	}
	if outputTokens < 0 {
		outputTokens = 0
	}
	return Work{terms: [MaxWorkTerms]float64{inputTokens, outputTokens}, arity: 2}
}

// Arity reports how many terms are meaningful. Zero means unsized.
func (work Work) Arity() int {
	return work.arity
}

// Term returns the value of one regressor. It reports 0 for an index at or
// beyond Arity, which is always the mathematically correct value to fold into a
// prediction of the wrong shape — callers should still check Arity before using
// a Work at all.
func (work Work) Term(index int) float64 {
	if index < 0 || index >= work.arity {
		return 0
	}
	return work.terms[index]
}

// Terms returns the meaningful terms as a slice, for the wire form and for
// building moment sums generically over any arity.
func (work Work) Terms() []float64 {
	if work.arity <= 0 {
		return nil
	}
	result := make([]float64, work.arity)
	copy(result, work.terms[:work.arity])
	return result
}

// WorkFromTerms builds a Work from its wire form. It reports false for an empty
// or over-long slice rather than silently truncating or padding.
func WorkFromTerms(terms []float64) (Work, bool) {
	if len(terms) == 0 || len(terms) > MaxWorkTerms {
		return Work{}, false
	}
	work := Work{arity: len(terms)}
	copy(work.terms[:], terms)
	return work, true
}

// Positive reports whether this Work is sized and every term is non-negative
// with at least one strictly positive — the shape a real request's size takes.
func (work Work) Positive() bool {
	if work.arity <= 0 {
		return false
	}
	anyPositive := false
	for index := 0; index < work.arity; index++ {
		if work.terms[index] < 0 {
			return false
		}
		if work.terms[index] > 0 {
			anyPositive = true
		}
	}
	return anyPositive
}

// Add sums two Work vectors of matching arity. An unsized (zero-arity) operand
// acts as the identity element rather than forcing a mismatch, which is what
// lets an accumulator start from the Work zero value and take on whichever
// arity its first real entry has. Two vectors of different nonzero arity report
// false rather than adding across different regressors.
func (work Work) Add(other Work) (Work, bool) {
	if work.arity == 0 {
		return other, true
	}
	if other.arity == 0 {
		return work, true
	}
	if work.arity != other.arity {
		return Work{}, false
	}
	result := Work{arity: work.arity}
	for index := 0; index < work.arity; index++ {
		result.terms[index] = work.terms[index] + other.terms[index]
	}
	return result, true
}

// Scaled multiplies every term by factor, keeping the same arity. Used to turn a
// summed backlog into a mean single-request size.
func (work Work) Scaled(factor float64) Work {
	result := Work{arity: work.arity}
	for index := 0; index < work.arity; index++ {
		result.terms[index] = work.terms[index] * factor
	}
	return result
}

// MarshalJSON and UnmarshalJSON round-trip Work as its Terms slice, since
// terms and arity are unexported: without this a Work embedded in a published
// status silently marshals as an empty object and a node's backlog magnitude
// never reaches the master at all.
func (work Work) MarshalJSON() ([]byte, error) {
	return json.Marshal(work.Terms())
}

func (work *Work) UnmarshalJSON(data []byte) error {
	var terms []float64
	if err := json.Unmarshal(data, &terms); err != nil {
		return err
	}
	if len(terms) == 0 {
		*work = Work{}
		return nil
	}
	decoded, ok := WorkFromTerms(terms)
	if !ok {
		*work = Work{}
		return nil
	}
	*work = decoded
	return nil
}
