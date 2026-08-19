// Package features converts raw price series into stationary inputs suitable
// for decision-tree ensembles. Raw nominal prices are never fed to the model;
// only transformed, scale-invariant values are.
package features

import "math"

// minSampleSize is the minimum number of values required to compute a standard deviation.
const minSampleSize = 2

// Mean returns the arithmetic mean of values.
func Mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0

	for _, v := range values {
		sum += v
	}

	return sum / float64(len(values))
}

// StdDev returns the sample standard deviation of values given a pre-computed mean.
func StdDev(values []float64, mean float64) float64 {
	if len(values) < minSampleSize {
		return 0
	}

	variance := 0.0

	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}

	return math.Sqrt(variance / float64(len(values)))
}
