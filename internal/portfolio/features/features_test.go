package features

import (
	"math"
	"testing"
)

func TestMean(t *testing.T) {
	t.Parallel()
	if got := Mean([]float64{1, 2, 3, 4, 5}); math.Abs(got-3.0) > 1e-9 {
		t.Errorf("Mean = %f, want 3.0", got)
	}
	if got := Mean(nil); got != 0 {
		t.Errorf("Mean(nil) = %f, want 0", got)
	}
}

func TestStdDev(t *testing.T) {
	t.Parallel()
	vals := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	mean := Mean(vals)
	std := StdDev(vals, mean)
	if math.Abs(std-2.0) > 0.001 {
		t.Errorf("StdDev = %f, want ~2.0", std)
	}
	if got := StdDev([]float64{1}, 1); got != 0 {
		t.Errorf("StdDev single element = %f, want 0", got)
	}
}

func TestZScore(t *testing.T) {
	t.Parallel()
	vals := []float64{1, 2, 3, 4, 5}
	mean := Mean(vals)
	std := StdDev(vals, mean)
	z := ZScore(vals, 3)
	expected := (3 - mean) / std
	if math.Abs(z-expected) > 1e-9 {
		t.Errorf("ZScore = %f, want %f", z, expected)
	}
	if got := ZScore(nil, 1); got != 0 {
		t.Errorf("ZScore(nil) = %f, want 0", got)
	}
}

func TestRelativeDistance(t *testing.T) {
	t.Parallel()
	if got := RelativeDistance(110, 100); math.Abs(got-0.1) > 1e-9 {
		t.Errorf("RelativeDistance = %f, want 0.1", got)
	}
	if got := RelativeDistance(90, 100); math.Abs(got-(-0.1)) > 1e-9 {
		t.Errorf("RelativeDistance = %f, want -0.1", got)
	}
	if got := RelativeDistance(5, 0); got != 0 {
		t.Errorf("RelativeDistance(x, 0) = %f, want 0", got)
	}
}

func TestPctReturn(t *testing.T) {
	t.Parallel()
	if got := PctReturn(100, 110); math.Abs(got-0.1) > 1e-9 {
		t.Errorf("PctReturn = %f, want 0.1", got)
	}
	if got := PctReturn(0, 100); got != 0 {
		t.Errorf("PctReturn(0, 100) = %f, want 0", got)
	}
}

func TestRollingReturns(t *testing.T) {
	t.Parallel()
	prices := []float64{100, 110, 99}
	returns := RollingReturns(prices)
	if len(returns) != 2 {
		t.Fatalf("len(returns) = %d, want 2", len(returns))
	}
	if math.Abs(returns[0]-0.1) > 1e-9 {
		t.Errorf("returns[0] = %f, want 0.1", returns[0])
	}
	if math.Abs(returns[1]-(99-110)/110.0) > 1e-9 {
		t.Errorf("returns[1] = %f, want %f", returns[1], (99-110)/110.0)
	}
	if RollingReturns([]float64{1}) != nil {
		t.Error("RollingReturns single element should be nil")
	}
}

func TestNormalisedRSI(t *testing.T) {
	t.Parallel()
	if got := NormalisedRSI(25, 25, 75); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("at lower threshold: %f, want 1.0", got)
	}
	if got := NormalisedRSI(75, 25, 75); math.Abs(got-(-1.0)) > 1e-9 {
		t.Errorf("at upper threshold: %f, want -1.0", got)
	}
	if got := NormalisedRSI(50, 25, 75); got != 0 {
		t.Errorf("at mid: %f, want 0", got)
	}
}

func TestClamp(t *testing.T) {
	t.Parallel()
	if Clamp(2, 0, 1) != 1 {
		t.Error("Clamp above max failed")
	}
	if Clamp(-1, 0, 1) != 0 {
		t.Error("Clamp below min failed")
	}
	if Clamp(0.5, 0, 1) != 0.5 {
		t.Error("Clamp within range failed")
	}
}

func TestRollingZScoreWindowExcludesCurrent(t *testing.T) {
	t.Parallel()
	series := make([]float64, 300)
	for i := range series {
		series[i] = float64(i + 1)
	}
	// Score at idx=252 using a 252-day window → reference is series[0:252].
	z1 := RollingZScore(series, 252, 252)

	// Mutating a future value (past idx=252) must NOT change the score.
	series[299] = 1e9
	z2 := RollingZScore(series, 252, 252)
	if z1 != z2 {
		t.Errorf("RollingZScore changed after mutating a future value: z1=%f z2=%f", z1, z2)
	}

	// Mutating a value inside the window (idx<252) MUST change the score.
	series[100] = 1e9
	z3 := RollingZScore(series, 252, 252)
	if z3 == z1 {
		t.Error("RollingZScore did not react to a change inside the rolling window")
	}
}

func TestRollingZScoreEdgeCases(t *testing.T) {
	t.Parallel()
	series := []float64{1, 2, 3, 4, 5}

	if RollingZScore(series, 0, 4) != 0 {
		t.Error("RollingZScore at idx=0 should be 0 (no history)")
	}
	if RollingZScore(series, 1, 4) != 0 {
		t.Error("RollingZScore with only 1 preceding sample should be 0")
	}

	// idx=4, window=3 → hist = series[1:4] = [2,3,4], scoring series[4]=5
	hist := []float64{2, 3, 4}
	m := Mean(hist)
	s := StdDev(hist, m)
	want := (5 - m) / s
	if got := RollingZScore(series, 4, 3); math.Abs(got-want) > 1e-9 {
		t.Errorf("RollingZScore(series, 4, 3) = %f, want %f", got, want)
	}
}

func TestMomentumNoLookahead(t *testing.T) {
	t.Parallel()
	series := make([]float64, 300)
	for i := range series {
		series[i] = 100 + float64(i)
	}
	// Momentum at idx=252, window=252 → PctReturn(series[0], series[252])
	m1 := Momentum(series, 252, 252)

	// Future values must not affect the result.
	series[299] = 1e9
	m2 := Momentum(series, 252, 252)
	if m1 != m2 {
		t.Errorf("Momentum changed after mutating a future value: m1=%f m2=%f", m1, m2)
	}

	// Changing the reference value (series[0]) must change the result.
	series[0] = 1e9
	m3 := Momentum(series, 252, 252)
	if m3 == m1 {
		t.Error("Momentum should react to a change in the reference (lagged) value")
	}
}

func TestMomentumEdgeCases(t *testing.T) {
	t.Parallel()
	series := []float64{100, 110, 121}

	// idx=2, window=2 → PctReturn(series[0], series[2]) = (121-100)/100 = 0.21
	if got := Momentum(series, 2, 2); math.Abs(got-0.21) > 1e-9 {
		t.Errorf("Momentum(series, 2, 2) = %f, want 0.21", got)
	}
	if got := Momentum(series, 1, 2); got != 0 {
		t.Errorf("Momentum with idx < window = %f, want 0", got)
	}
	if got := Momentum(series, 0, 0); got != 0 {
		t.Errorf("Momentum with window=0 = %f, want 0", got)
	}
	if got := Momentum(series, 100, 1); got != 0 {
		t.Errorf("Momentum with out-of-bounds idx = %f, want 0", got)
	}
}

func TestExponentialSmoother(t *testing.T) {
	t.Parallel()
	s := NewExponentialSmoother(1.0) // α=1 → passthrough
	if got := s.Update(42); got != 42 {
		t.Errorf("α=1 passthrough: %f, want 42", got)
	}

	s2 := NewExponentialSmoother(0.5)
	s2.Update(100)
	v := s2.Update(0)
	if math.Abs(v-50) > 1e-9 {
		t.Errorf("EMA step: %f, want 50", v)
	}

	s2.Reset()
	if s2.Value() != 0 {
		t.Error("Reset should zero the smoother")
	}
}

func TestSmoothedReturns(t *testing.T) {
	t.Parallel()
	returns := []float64{0.1, -0.1, 0.1}
	smoothed := SmoothedReturns(returns, 0.5)
	if len(smoothed) != len(returns) {
		t.Fatalf("len mismatch: got %d, want %d", len(smoothed), len(returns))
	}
	if smoothed[0] != 0.1 {
		t.Errorf("first smoothed value = %f, want 0.1", smoothed[0])
	}
}

func TestNewExponentialSmootherClamps(t *testing.T) {
	t.Parallel()
	s := NewExponentialSmoother(-5)
	s.Update(1)
	// Should not panic; alpha clamped to 0.01
	if s.Value() == 0 {
		t.Error("smoother should have a non-zero value after update")
	}
	s2 := NewExponentialSmoother(5)
	v := s2.Update(100)
	if v != 100 { // alpha clamped to 1 = passthrough
		t.Errorf("alpha>1 clamped incorrectly: %f", v)
	}
}
