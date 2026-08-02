package ml

import (
	"math"
	"testing"
)

func TestForestFitPredict(t *testing.T) {
	t.Parallel()
	samples := makeSamples(300)
	f := NewForest(10, 4, 3)
	f.Fit(samples, 10)

	if len(f.Trees) != 10 {
		t.Errorf("expected 10 trees, got %d", len(f.Trees))
	}

	var feat [featureCount]float64
	feat[FeatRSI] = 0.8 // strong buy signal
	pred := f.Predict(feat)
	if math.IsNaN(pred) || math.IsInf(pred, 0) {
		t.Errorf("Predict invalid: %f", pred)
	}
}

func TestForestEmptyPredict(t *testing.T) {
	t.Parallel()
	f := NewForest(5, 3, 2)
	// No Fit called
	pred := f.Predict([featureCount]float64{})
	if pred != 0 {
		t.Errorf("empty forest Predict = %f, want 0", pred)
	}
}

func TestConvictionScore(t *testing.T) {
	t.Parallel()
	samples := makeSamples(200)
	f := NewForest(5, 3, 2)
	f.Fit(samples, 5)

	var feat [featureCount]float64
	feat[FeatRSI] = 1.0
	score := f.ConvictionScore(feat, 0.1)
	if score < -1 || score > 1 {
		t.Errorf("ConvictionScore out of range: %f", score)
	}
}

func TestConvictionScoreZeroScale(t *testing.T) {
	t.Parallel()
	f := NewForest(3, 2, 2)
	f.Fit(makeSamples(50), 3)
	score := f.ConvictionScore([featureCount]float64{}, 0) // zero scale → uses 0.01
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

func TestSharpe(t *testing.T) {
	t.Parallel()
	// Constant positive returns → very high Sharpe.
	returns := make([]float64, 252)
	for i := range returns {
		returns[i] = 0.001
	}
	s := Sharpe(returns)
	if s <= 0 {
		t.Errorf("Sharpe for positive constant returns = %f, want > 0", s)
	}
	if got := Sharpe([]float64{1}); got != 0 {
		t.Errorf("Sharpe single return = %f, want 0", got)
	}
}

func TestLCG(t *testing.T) {
	t.Parallel()
	rng := newLCG(1)
	seen := make(map[uint64]bool)
	for i := 0; i < 1000; i++ {
		v := rng.next()
		if seen[v] {
			t.Logf("collision at iteration %d (value %d)", i, v)
		}
		seen[v] = true
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
	for _, fi := range mask {
		if fi < 0 || fi >= featureCount {
			t.Errorf("invalid feature index %d", fi)
		}
	}
}
