// Package features converts raw price series into stationary inputs suitable
// for decision-tree ensembles. Raw nominal prices are never fed to the model;
// only transformed, scale-invariant values are.
package features

import "math"

// ZScore returns the standard-score of value relative to the provided slice.
// Returns 0 when the slice has fewer than 2 elements or zero standard deviation.
func ZScore(values []float64, value float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := Mean(values)
	std := StdDev(values, mean)
	if std == 0 {
		return 0
	}
	return (value - mean) / std
}

// RelativeDistance returns (value - reference) / reference.
// Returns 0 when reference is zero to avoid division by zero.
func RelativeDistance(value, reference float64) float64 {
	if reference == 0 {
		return 0
	}
	return (value - reference) / reference
}

// PctReturn computes the percentage return between prev and curr.
// Returns 0 when prev is zero.
func PctReturn(prev, curr float64) float64 {
	if prev == 0 {
		return 0
	}
	return (curr - prev) / prev
}

// RollingReturns computes percentage returns for a sliding window of prices.
// Returns an empty slice when prices has fewer than 2 elements.
func RollingReturns(prices []float64) []float64 {
	if len(prices) < 2 {
		return nil
	}
	out := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		out[i-1] = PctReturn(prices[i-1], prices[i])
	}
	return out
}

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
	if len(values) < 2 {
		return 0
	}
	variance := 0.0
	for _, v := range values {
		diff := v - mean
		variance += diff * diff
	}
	return math.Sqrt(variance / float64(len(values)))
}

// NormalisedRSI maps an RSI value in [0,100] to [-1,+1] using a linear
// piecewise function centred on 50.
func NormalisedRSI(rsiVal, lower, upper float64) float64 {
	const mid = 50.0
	switch {
	case rsiVal <= lower:
		return 1.0
	case rsiVal >= upper:
		return -1.0
	case rsiVal < mid:
		if mid-lower == 0 {
			return 0
		}
		return (mid - rsiVal) / (mid - lower)
	default:
		if upper-mid == 0 {
			return 0
		}
		return -(rsiVal - mid) / (upper - mid)
	}
}

// RollingZScore scores series[idx] against the preceding window observations
// series[max(0,idx-window) : idx], strictly excluding series[idx] itself.
// This enforces a t-1 cutoff and eliminates lookahead bias: future values and
// the scored value never influence the reference distribution.
// Returns 0 when fewer than 2 in-window samples precede idx.
func RollingZScore(series []float64, idx, window int) float64 {
	if idx <= 0 || idx >= len(series) || window <= 0 {
		return 0
	}
	start := idx - window
	if start < 0 {
		start = 0
	}
	hist := series[start:idx]
	if len(hist) < 2 {
		return 0
	}
	mean := Mean(hist)
	std := StdDev(hist, mean)
	if std == 0 {
		return 0
	}
	return (series[idx] - mean) / std
}

// Momentum returns the percentage return of series[idx] relative to
// series[idx-window]. The reference is strictly in the past (t-window) and
// the current value is scored without any future data influence.
// Returns 0 when idx < window, window ≤ 0, or idx is out of bounds.
func Momentum(series []float64, idx, window int) float64 {
	if window <= 0 || idx < window || idx >= len(series) {
		return 0
	}
	return PctReturn(series[idx-window], series[idx])
}

// Clamp restricts v to [min, max].
func Clamp(v, minVal, maxVal float64) float64 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}
