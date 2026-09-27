package proxy

import (
	"errors"
	"testing"
)

func TestDeliveryFailuresReportOnlyChanges(t *testing.T) {
	failures := newDeliveryFailures()
	refused := errors.New("status 404")

	outcomes := []bool{
		failures.outcomeChanged("slave-1", nil),
		failures.outcomeChanged("slave-1", refused),
		failures.outcomeChanged("slave-1", refused),
		failures.outcomeChanged("slave-2", refused),
		failures.outcomeChanged("slave-1", nil),
	}

	if want := []bool{false, true, false, true, true}; !equalBools(outcomes, want) {
		t.Fatalf("changed = %v, want %v", outcomes, want)
	}
}

func equalBools(left []bool, right []bool) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
