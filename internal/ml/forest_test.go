//nolint:testpackage // accesses unexported symbols: newLCG, bootstrap, randomFeatureMask, featureCount, FeatRSI
package ml

import (
	"math"
	"testing"
)

func TestForestFitPredict(t *testing.T) {
	t.Parallel()

	samples := makeSamples(300)
	forest := NewForest(10, 4, 3, 0)
	forest.Fit(samples, 10)

	if len(forest.Trees) != 10 {
		t.Errorf("expected 10 trees, got %d", len(forest.Trees))
	}

	var feat [featureCount]float64

	feat[FeatRSI] = 0.8 // strong buy signal
	pred := forest.Predict(feat)

	if math.IsNaN(pred) || math.IsInf(pred, 0) {
		t.Errorf("Predict invalid: %f", pred)
	}
}

func TestForestEmptyPredict(t *testing.T) {
	t.Parallel()

	forest := NewForest(5, 3, 2, 0)
	// No Fit called
	pred := forest.Predict([featureCount]float64{})

	if pred != 0 {
		t.Errorf("empty forest Predict = %f, want 0", pred)
	}
}

func TestConvictionScore(t *testing.T) {
	t.Parallel()

	samples := makeSamples(200)
	forest := NewForest(5, 3, 2, 0)
	forest.Fit(samples, 5)

	var feat [featureCount]float64

	feat[FeatRSI] = 1.0
	score := forest.ConvictionScore(feat, 0.1)

	if score < -1 || score > 1 {
		t.Errorf("ConvictionScore out of range: %f", score)
	}
}

func TestConvictionScoreZeroScale(t *testing.T) {
	t.Parallel()

	forest := NewForest(3, 2, 2, 0)
	forest.Fit(makeSamples(50), 3)
	score := forest.ConvictionScore([featureCount]float64{}, 0) // zero scale → uses 0.01

	if math.IsNaN(score) {
		t.Error("ConvictionScore(scale=0) is NaN")
	}
}

func TestMSEMAE(t *testing.T) {
	t.Parallel()

	preds := []float64{1, 2, 3}
	labels := []float64{1, 2, 3}

	if got := MSE(preds, labels); got != 0 {
		t.Errorf("MSE perfect predictions = %f, want 0", got)
	}

	if got := MAE(preds, labels); got != 0 {
		t.Errorf("MAE perfect predictions = %f, want 0", got)
	}

	if got := MSE(nil, nil); got != 0 {
		t.Errorf("MSE(nil) = %f, want 0", got)
	}
}

func TestSortino(t *testing.T) {
	t.Parallel()

	// Constant positive returns → no downside → capped value.
	returns := make([]float64, tradingDaysPerYear)
	for i := range returns {
		returns[i] = 0.001
	}

	sortVal := Sortino(returns)
	if sortVal != maxSortinoRatio {
		t.Errorf("Sortino for positive constant returns = %f, want %f (no downside)", sortVal, maxSortinoRatio)
	}

	// Mixed returns with negatives → finite positive ratio.
	mixed := make([]float64, tradingDaysPerYear)
	for i := range mixed {
		if i%3 == 0 {
			mixed[i] = -0.001
		} else {
			mixed[i] = 0.002
		}
	}

	sm := Sortino(mixed)
	if sm <= 0 {
		t.Errorf("Sortino for mixed returns = %f, want > 0", sm)
	}

	if got := Sortino([]float64{1}); got != 0 {
		t.Errorf("Sortino single return = %f, want 0", got)
	}

	// All negative returns → mean < 0, dd > 0 → valid negative Sortino.
	neg := []float64{-0.01, -0.02, -0.01}
	if got := Sortino(neg); got >= 0 {
		t.Errorf("Sortino all-negative returns = %f, want < 0", got)
	}
}

func TestLCG(t *testing.T) {
	t.Parallel()

	rng := newLCG(1)
	seen := make(map[uint64]bool)

	for i := range 1000 {
		lcgVal := rng.next()
		if seen[lcgVal] {
			t.Logf("collision at iteration %d (value %d)", i, lcgVal)
		}

		seen[lcgVal] = true
	}
}

func TestBootstrap(t *testing.T) {
	t.Parallel()

	samples := makeSamples(50)
	rng := newLCG(7)
	boot := bootstrap(samples, rng)

	if len(boot) != len(samples) {
		t.Errorf("bootstrap len = %d, want %d", len(boot), len(samples))
	}
}

func TestRandomFeatureMask(t *testing.T) {
	t.Parallel()

	rng := newLCG(3)
	mask := randomFeatureMask(featureCount, 4, rng)

	if len(mask) != 4 {
		t.Errorf("mask len = %d, want 4", len(mask))
	}

	// All indices should be in valid range.
	for _, featIdx := range mask {
		if featIdx < 0 || featIdx >= featureCount {
			t.Errorf("invalid feature index %d", featIdx)
		}
	}
}
