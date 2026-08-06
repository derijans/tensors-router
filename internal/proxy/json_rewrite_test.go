package proxy

import (
	"math"
	"testing"
)

func TestReplacementBufferCapacity(t *testing.T) {
	tests := []struct {
		name              string
		originalLength    int
		replacementLength int
		replacedLength    int
		wantCapacity      int
		wantOK            bool
	}{
		{name: "growing replacement", originalLength: 20, replacementLength: 12, replacedLength: 4, wantCapacity: 28, wantOK: true},
		{name: "shrinking replacement", originalLength: 20, replacementLength: 2, replacedLength: 4, wantCapacity: 18, wantOK: true},
		{name: "maximum capacity", originalLength: math.MaxInt, replacementLength: 1, replacedLength: 1, wantCapacity: math.MaxInt, wantOK: true},
		{name: "overflow", originalLength: math.MaxInt, replacementLength: 2, replacedLength: 1, wantOK: false},
		{name: "invalid replaced length", originalLength: 20, replacementLength: 2, replacedLength: 21, wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			capacity, ok := replacementBufferCapacity(test.originalLength, test.replacementLength, test.replacedLength)
			if capacity != test.wantCapacity || ok != test.wantOK {
				t.Fatalf("replacementBufferCapacity() = (%d, %t), want (%d, %t)", capacity, ok, test.wantCapacity, test.wantOK)
			}
		})
	}
}
