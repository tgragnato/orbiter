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
	// LabelHorizon is the number of forward bars the label spans (e.g. 5 for a
	// 5-day forward return). purge uses this to strip the trailing LabelHorizon
	// training samples whose forward-return labels bleed into the test window.
	// A value of 0 disables purging (safe only when Embargo >= LabelHorizon).
	LabelHorizon int
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

		trainSamples := purge(samples[trainStart:trainEnd], trainStart, testStart, cfg.LabelHorizon)
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
			Fold:    fold,
			MSE:     MSE(preds, labels),
			MAE:     MAE(preds, labels),
			Sortino: Sortino(returns),
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

// purge removes training samples whose forward-return label bleeds into the
// test window. Sample at position j in trainSamples has absolute index
// trainStart+j and a label that closes at trainStart+j+labelHorizon. It leaks
// if trainStart+j+labelHorizon >= testStart, i.e. j >= testStart-labelHorizon-trainStart.
// When labelHorizon is 0 or the embargo already covers the horizon, nothing is removed.
func purge(trainSamples []Sample, trainStart, testStart, labelHorizon int) []Sample {
	if labelHorizon <= 0 {
		return trainSamples
	}
	cutoff := testStart - labelHorizon - trainStart
	if cutoff <= 0 {
		return nil
	}
	if cutoff >= len(trainSamples) {
		return trainSamples
	}
	return trainSamples[:cutoff]
}

// BestFold returns the fold with the highest Sortino ratio.
// Used for diagnostic logging only — not for live inference (see MergeForests).
func BestFold(results []WalkForwardResult) *WalkForwardResult {
	if len(results) == 0 {
		return nil
	}
	best := &results[0]
	for i := 1; i < len(results); i++ {
		if results[i].Metrics.Sortino > best.Metrics.Sortino {
			best = &results[i]
		}
	}
	return best
}

// MergeForests combines all fold forests into a single Forest by concatenating
// their trees. The merged ensemble averages predictions across every fold,
// eliminating fold-selection bias that arises from picking the single highest
// OOS Sortino ratio (which reflects noise in a 60-day window, not robustness).
func MergeForests(results []WalkForwardResult) *Forest {
	if len(results) == 0 {
		return nil
	}
	merged := &Forest{}
	for _, r := range results {
		if r.Forest == nil || len(r.Forest.Trees) == 0 {
			continue
		}
		if merged.nFeatures == 0 {
			merged.nFeatures = r.Forest.nFeatures
			merged.maxDepth = r.Forest.maxDepth
			merged.minSamples = r.Forest.minSamples
		}
		merged.Trees = append(merged.Trees, r.Forest.Trees...)
	}
	if len(merged.Trees) == 0 {
		return nil
	}
	return merged
}
