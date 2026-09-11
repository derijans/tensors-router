package schedulingcost

import "math"

// minPivotRatio guards against a near-singular system. Prefill and decode
// tokens correlate strongly in real chat traffic (long prompts get long
// answers), and a collinear system is technically solvable but the split
// between the two slopes is noise dressed as a measurement. scale is the
// largest absolute entry of the original moment matrix, so the threshold
// adapts to the units of the data instead of comparing against a fixed number.
const minPivotRatio = 1e-9

// solveNormalEquations solves moments*beta = targets by Gaussian elimination
// with partial pivoting, rejecting rather than returning an unstable answer
// when a pivot collapses relative to the matrix's own scale. moments must be
// square; targets must have the same length.
func solveNormalEquations(moments [][]float64, targets []float64) ([]float64, bool) {
	n := len(targets)
	if n == 0 || len(moments) != n {
		return nil, false
	}
	scale := 0.0
	augmented := make([][]float64, n)
	for row := 0; row < n; row++ {
		if len(moments[row]) != n {
			return nil, false
		}
		augmented[row] = make([]float64, n+1)
		copy(augmented[row], moments[row])
		augmented[row][n] = targets[row]
		for col := 0; col < n; col++ {
			if abs := math.Abs(moments[row][col]); abs > scale {
				scale = abs
			}
		}
	}
	if scale == 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return nil, false
	}
	threshold := minPivotRatio * scale

	for pivotIndex := 0; pivotIndex < n; pivotIndex++ {
		bestRow := pivotIndex
		bestValue := math.Abs(augmented[pivotIndex][pivotIndex])
		for row := pivotIndex + 1; row < n; row++ {
			if value := math.Abs(augmented[row][pivotIndex]); value > bestValue {
				bestRow, bestValue = row, value
			}
		}
		if bestValue < threshold {
			return nil, false
		}
		augmented[pivotIndex], augmented[bestRow] = augmented[bestRow], augmented[pivotIndex]

		pivot := augmented[pivotIndex][pivotIndex]
		for row := pivotIndex + 1; row < n; row++ {
			factor := augmented[row][pivotIndex] / pivot
			if factor == 0 {
				continue
			}
			for col := pivotIndex; col <= n; col++ {
				augmented[row][col] -= factor * augmented[pivotIndex][col]
			}
		}
	}

	solution := make([]float64, n)
	for row := n - 1; row >= 0; row-- {
		sum := augmented[row][n]
		for col := row + 1; col < n; col++ {
			sum -= augmented[row][col] * solution[col]
		}
		value := sum / augmented[row][row]
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, false
		}
		solution[row] = value
	}
	return solution, true
}
