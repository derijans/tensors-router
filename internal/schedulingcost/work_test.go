package schedulingcost

import (
	"encoding/json"
	"testing"
)

func TestWorkRoundTripsThroughJSON(t *testing.T) {
	original := TextWork(120, 45)
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Work
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Arity() != original.Arity() || decoded.Term(0) != original.Term(0) || decoded.Term(1) != original.Term(1) {
		t.Fatalf("round trip = %+v, want %+v", decoded, original)
	}
}

func TestZeroWorkRoundTripsThroughJSON(t *testing.T) {
	var zero Work
	encoded, err := json.Marshal(zero)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Work
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Arity() != 0 {
		t.Fatalf("decoded arity = %d, want 0", decoded.Arity())
	}
}

func TestAddTreatsZeroArityAsIdentity(t *testing.T) {
	sum, ok := Work{}.Add(ImageWork(100))
	if !ok || sum.Arity() != 1 || sum.Term(0) != 100 {
		t.Fatalf("zero + work = %+v ok=%t, want the work operand unchanged", sum, ok)
	}
	sum, ok = ImageWork(100).Add(Work{})
	if !ok || sum.Arity() != 1 || sum.Term(0) != 100 {
		t.Fatalf("work + zero = %+v ok=%t, want the work operand unchanged", sum, ok)
	}
}

func TestAddRejectsMismatchedNonzeroArity(t *testing.T) {
	if _, ok := ImageWork(100).Add(TextWork(10, 20)); ok {
		t.Fatal("added an image work vector to a text work vector")
	}
}
