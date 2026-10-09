package schedulingcost

import "math"

const minPivotToMatrixScaleRatio = 1e-9

func solveNormalEquations(moments [][]float64, targets []float64) ([]float64, bool) {
	n := len(targets)
	if n == 0 || len(moments) != n {
		return nil, false
	}
	augmented := make([][]float64, n)
	for row := 0; row < n; row++ {
		if len(moments[row]) != n {
			return nil, false
		}
		augmented[row] = make([]float64, n+1)
		copy(augmented[row], moments[row])
		augmented[row][n] = targets[row]
	}
	scale := largestAbsoluteEntry(moments)
	if scale == 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return nil, false
	}
	threshold := minPivotToMatrixScaleRatio * scale

	if !eliminateWithPartialPivoting(augmented, threshold) {
		return nil, false
	}
	return backSubstitute(augmented)
}

func largestAbsoluteEntry(matrix [][]float64) float64 {
	largest := 0.0
	for _, row := range matrix {
		for _, value := range row {
			if abs := math.Abs(value); abs > largest {
				largest = abs
			}
		}
	}
	return largest
}

func eliminateWithPartialPivoting(augmented [][]float64, threshold float64) bool {
	for pivotIndex := range augmented {
		bestRow, bestValue := largestPivotRow(augmented, pivotIndex)
		if bestValue < threshold {
			return false
		}
		augmented[pivotIndex], augmented[bestRow] = augmented[bestRow], augmented[pivotIndex]
		eliminateBelowPivot(augmented, pivotIndex)
	}
	return true
}

func largestPivotRow(augmented [][]float64, pivotIndex int) (int, float64) {
	bestRow := pivotIndex
	bestValue := math.Abs(augmented[pivotIndex][pivotIndex])
	for row := pivotIndex + 1; row < len(augmented); row++ {
		if value := math.Abs(augmented[row][pivotIndex]); value > bestValue {
			bestRow, bestValue = row, value
		}
	}
	return bestRow, bestValue
}

func eliminateBelowPivot(augmented [][]float64, pivotIndex int) {
	pivotRow := augmented[pivotIndex]
	pivot := pivotRow[pivotIndex]
	for _, row := range augmented[pivotIndex+1:] {
		factor := row[pivotIndex] / pivot
		if factor == 0 {
			continue
		}
		for col := pivotIndex; col < len(row); col++ {
			row[col] -= factor * pivotRow[col]
		}
	}
}

func backSubstitute(augmented [][]float64) ([]float64, bool) {
	n := len(augmented)
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
