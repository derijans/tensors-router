package schedulingcost

import "encoding/json"

const MaxWorkTerms = 2

type Work struct {
	terms [MaxWorkTerms]float64
	arity int
}

func ImageWork(pixelSteps float64) Work {
	if pixelSteps <= 0 {
		return Work{}
	}
	return Work{terms: [MaxWorkTerms]float64{pixelSteps}, arity: 1}
}

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

func (work Work) Arity() int {
	return work.arity
}

func (work Work) Term(index int) float64 {
	if index < 0 || index >= work.arity {
		return 0
	}
	return work.terms[index]
}

func (work Work) Terms() []float64 {
	if work.arity <= 0 {
		return nil
	}
	result := make([]float64, work.arity)
	copy(result, work.terms[:work.arity])
	return result
}

func WorkFromTerms(terms []float64) (Work, bool) {
	if len(terms) == 0 || len(terms) > MaxWorkTerms {
		return Work{}, false
	}
	work := Work{arity: len(terms)}
	copy(work.terms[:], terms)
	return work, true
}

func (work Work) IsMeasuredSize() bool {
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

func (work Work) Scaled(factor float64) Work {
	result := Work{arity: work.arity}
	for index := 0; index < work.arity; index++ {
		result.terms[index] = work.terms[index] * factor
	}
	return result
}

func (work Work) Mean(count int64) Work {
	if count <= 0 {
		return Work{}
	}
	return work.Scaled(1 / float64(count))
}

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
