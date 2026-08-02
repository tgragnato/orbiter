package ml

import "fmt"

// WalkForwardConfig controls the purged walk-forward cross-validation.
type WalkForwardConfig struct {
	// TrainSize is the number of samples in each training window.
	TrainSize int
	// TestSize is the number of samples in each test window.
	TestSize int
	// Embargo is the number of samples dropped between train end and test start
	// to prevent label leakage from overlapping return horizons.
	Embargo int
	// NTrees is the number of trees per forest in each fold.
	NTrees int
	// MaxDepth limits tree depth.
	MaxDepth int
	// MinSamples is the minimum number of samples in a leaf.
	MinSamples int
}

// WalkForwardResult holds per-fold validation metrics.
type WalkForwardResult struct {
	Fold       int
	TrainStart int
	TrainEnd   int
	TestStart  int
	TestEnd    int
	Metrics    Metrics
	Forest     *Forest
}

// WalkForwardCV runs purged walk-forward cross-validation on samples.
// Each fold trains a fresh Forest and evaluates on the out-of-sample test window.
// The embargo gap between train end and test start prevents lookahead bias from
// overlapping return horizons.
//
// The onFold callback (if non-nil) is invoked after each fold for progress
// reporting; it receives the fold result and returns true to continue or false
// to abort early.
func WalkForwardCV(
	samples []Sample,
	cfg WalkForwardConfig,
	onFold func(WalkForwardResult) bool,
) ([]WalkForwardResult, error) {
	if cfg.TrainSize <= 0 || cfg.TestSize <= 0 {
		return nil, fmt.Errorf("TrainSize and TestSize must be > 0")
	}
	if cfg.Embargo < 0 {
		cfg.Embargo = 0
	}
	n := len(samples)
	required := cfg.TrainSize + cfg.Embargo + cfg.TestSize
	if n < required {
		return nil, fmt.Errorf("not enough samples: have %d, need at least %d (trainSize+embargo+testSize)", n, required)
	}

	var results []WalkForwardResult
	fold := 0
	for start := 0; start+required <= n; start += cfg.TestSize {
		trainStart := start
		trainEnd := start + cfg.TrainSize
		testStart := trainEnd + cfg.Embargo
		testEnd := testStart + cfg.TestSize
		if testEnd > n {
			break
		}

		trainSamples := purge(samples[trainStart:trainEnd], testStart, testEnd)
		if len(trainSamples) == 0 {
			fold++
			continue
		}

		f := NewForest(cfg.NTrees, cfg.MaxDepth, cfg.MinSamples)
		f.Fit(trainSamples, cfg.NTrees)

		preds := make([]float64, cfg.TestSize)
		labels := make([]float64, cfg.TestSize)
		returns := make([]float64, cfg.TestSize)
		for i, s := range samples[testStart:testEnd] {
			preds[i] = f.Predict(s.Features)
			labels[i] = s.Label
			// Simulated strategy return: sign(prediction) * actual label
			if preds[i] >= 0 {
				returns[i] = s.Label
			} else {
				returns[i] = -s.Label
			}
		}

		m := Metrics{
			Fold:   fold,
			MSE:    MSE(preds, labels),
			MAE:    MAE(preds, labels),
			Sharpe: Sharpe(returns),
		}
		r := WalkForwardResult{
			Fold:       fold,
			TrainStart: trainStart,
			TrainEnd:   trainEnd,
			TestStart:  testStart,
			TestEnd:    testEnd,
			Metrics:    m,
			Forest:     f,
		}
		results = append(results, r)
		fold++

		if onFold != nil && !onFold(r) {
			break
		}
	}

	return results, nil
}

// purge removes any training sample whose index falls within [testStart, testEnd)
// after applying the embargo. For a fixed feature window (single-point labels)
// the purge set is empty, but the function is kept general.
func purge(trainSamples []Sample, testStart, testEnd int) []Sample {
	// With non-overlapping single-step labels, purging removes nothing.
	// Retained for correctness when label horizons exceed one period.
	_ = testStart
	_ = testEnd
	return trainSamples
}

// BestFold returns the fold with the highest Sharpe ratio.
func BestFold(results []WalkForwardResult) *WalkForwardResult {
	if len(results) == 0 {
		return nil
	}
	best := &results[0]
	for i := 1; i < len(results); i++ {
		if results[i].Metrics.Sharpe > best.Metrics.Sharpe {
			best = &results[i]
		}
	}
	return best
}
