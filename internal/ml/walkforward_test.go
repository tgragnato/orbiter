//nolint:testpackage // accesses unexported symbols: makeSamples, featureCount
package ml

import (
	"testing"
)

func TestWalkForwardCV(t *testing.T) {
	t.Parallel()

	samples := makeSamples(600)
	cfg := WalkForwardConfig{
		TrainSize:        200,
		TestSize:         50,
		Embargo:          5,
		LabelHorizon:     0,
		NTrees:           5,
		FeaturesPerSplit: 0,
		MaxDepth:         3,
		MinSamples:       5,
	}

	results, err := WalkForwardCV(samples, cfg, nil)
	if err != nil {
		t.Fatalf("WalkForwardCV error: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one fold result")
	}

	for _, result := range results {
		if result.Forest == nil {
			t.Errorf("fold %d: nil forest", result.Fold)
		}

		if result.TestEnd > len(samples) {
			t.Errorf("fold %d: testEnd %d > len(samples) %d", result.Fold, result.TestEnd, len(samples))
		}

		if result.TrainEnd > result.TestStart {
			t.Errorf("fold %d: trainEnd %d > testStart %d (no embargo gap)",
				result.Fold, result.TrainEnd, result.TestStart)
		}
	}
}

func TestWalkForwardCVEarlyAbort(t *testing.T) {
	t.Parallel()

	samples := makeSamples(600)
	cfg := WalkForwardConfig{
		TrainSize:        200,
		TestSize:         50,
		Embargo:          5,
		LabelHorizon:     0,
		NTrees:           3,
		FeaturesPerSplit: 0,
		MaxDepth:         2,
		MinSamples:       5,
	}
	abortAfter := 2
	foldCount := 0

	results, err := WalkForwardCV(samples, cfg, func(result WalkForwardResult) bool {
		foldCount++

		return foldCount < abortAfter
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) >= abortAfter+1 {
		t.Errorf("expected early abort: got %d folds, want ≤ %d", len(results), abortAfter)
	}
}

func TestWalkForwardCVNotEnoughSamples(t *testing.T) {
	t.Parallel()

	samples := makeSamples(10)
	cfg := WalkForwardConfig{
		TrainSize:        200,
		TestSize:         50,
		Embargo:          5,
		LabelHorizon:     0,
		NTrees:           3,
		FeaturesPerSplit: 0,
		MaxDepth:         3,
		MinSamples:       5,
	}

	_, err := WalkForwardCV(samples, cfg, nil)
	if err == nil {
		t.Error("expected error for insufficient samples, got nil")
	}
}

func TestWalkForwardCVInvalidConfig(t *testing.T) {
	t.Parallel()

	samples := makeSamples(100)
	cfg := WalkForwardConfig{
		TrainSize:        0,
		TestSize:         50,
		Embargo:          0,
		LabelHorizon:     0,
		NTrees:           0,
		FeaturesPerSplit: 0,
		MaxDepth:         0,
		MinSamples:       0,
	}

	_, err := WalkForwardCV(samples, cfg, nil)
	if err == nil {
		t.Error("expected error for TrainSize=0")
	}

	cfg2 := WalkForwardConfig{
		TrainSize:        50,
		TestSize:         0,
		Embargo:          0,
		LabelHorizon:     0,
		NTrees:           0,
		FeaturesPerSplit: 0,
		MaxDepth:         0,
		MinSamples:       0,
	}

	_, err = WalkForwardCV(samples, cfg2, nil)
	if err == nil {
		t.Error("expected error for TestSize=0")
	}
}

//nolint:funlen // comprehensive merge-forests test with multiple assertions
func TestMergeForests(t *testing.T) {
	t.Parallel()

	samples := makeSamples(600)
	cfg := WalkForwardConfig{
		TrainSize:        200,
		TestSize:         50,
		Embargo:          5,
		LabelHorizon:     0,
		NTrees:           5,
		FeaturesPerSplit: 0,
		MaxDepth:         3,
		MinSamples:       5,
	}

	results, err := WalkForwardCV(samples, cfg, nil)
	if err != nil {
		t.Fatalf("WalkForwardCV error: %v", err)
	}

	merged := MergeForests(results)
	if merged == nil {
		t.Fatal("MergeForests returned nil")

		return
	}

	totalTrees := 0

	for _, result := range results {
		if result.Forest != nil {
			totalTrees += len(result.Forest.Trees)
		}
	}

	if len(merged.Trees) != totalTrees {
		t.Errorf("merged tree count = %d, want %d", len(merged.Trees), totalTrees)
	}

	// Prediction must be finite.
	feat := samples[0].Features
	pred := merged.Predict(feat)

	if pred != pred { // NaN check
		t.Error("merged forest Predict returned NaN")
	}

	if got := MergeForests(nil); got != nil {
		t.Errorf("MergeForests(nil) = %v, want nil", got)
	}

	if got := MergeForests([]WalkForwardResult{
		{
			Fold:       0,
			TrainStart: 0,
			TrainEnd:   0,
			TestStart:  0,
			TestEnd:    0,
			Metrics:    Metrics{Fold: 0, MSE: 0, MAE: 0, Sortino: 0},
			Forest:     nil,
		},
	}); got != nil {
		t.Errorf("MergeForests with nil forests = %v, want nil", got)
	}
}

func TestBestFold(t *testing.T) {
	t.Parallel()

	results := []WalkForwardResult{
		{
			Fold:       0,
			TrainStart: 0,
			TrainEnd:   0,
			TestStart:  0,
			TestEnd:    0,
			Metrics:    Metrics{Fold: 0, MSE: 0, MAE: 0, Sortino: 0.5},
			Forest:     nil,
		},
		{
			Fold:       1,
			TrainStart: 0,
			TrainEnd:   0,
			TestStart:  0,
			TestEnd:    0,
			Metrics:    Metrics{Fold: 0, MSE: 0, MAE: 0, Sortino: 1.5},
			Forest:     nil,
		},
		{
			Fold:       2,
			TrainStart: 0,
			TrainEnd:   0,
			TestStart:  0,
			TestEnd:    0,
			Metrics:    Metrics{Fold: 0, MSE: 0, MAE: 0, Sortino: 1.0},
			Forest:     nil,
		},
	}
	best := BestFold(results)

	if best == nil || best.Fold != 1 {
		t.Errorf("BestFold = fold %v, want fold 1", best)
	}

	if got := BestFold(nil); got != nil {
		t.Errorf("BestFold(nil) = %v, want nil", got)
	}
}
