package ml

import (
	"testing"
)

func TestWalkForwardCV(t *testing.T) {
	t.Parallel()
	samples := makeSamples(600)
	cfg := WalkForwardConfig{
		TrainSize:  200,
		TestSize:   50,
		Embargo:    5,
		NTrees:     5,
		MaxDepth:   3,
		MinSamples: 5,
	}
	results, err := WalkForwardCV(samples, cfg, nil)
	if err != nil {
		t.Fatalf("WalkForwardCV error: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one fold result")
	}
	for _, r := range results {
		if r.Forest == nil {
			t.Errorf("fold %d: nil forest", r.Fold)
		}
		if r.TestEnd > len(samples) {
			t.Errorf("fold %d: testEnd %d > len(samples) %d", r.Fold, r.TestEnd, len(samples))
		}
		if r.TrainEnd > r.TestStart {
			t.Errorf("fold %d: trainEnd %d > testStart %d (no embargo gap)", r.Fold, r.TrainEnd, r.TestStart)
		}
	}
}

func TestWalkForwardCVEarlyAbort(t *testing.T) {
	t.Parallel()
	samples := makeSamples(600)
	cfg := WalkForwardConfig{
		TrainSize:  200,
		TestSize:   50,
		Embargo:    5,
		NTrees:     3,
		MaxDepth:   2,
		MinSamples: 5,
	}
	abortAfter := 2
	foldCount := 0
	results, err := WalkForwardCV(samples, cfg, func(r WalkForwardResult) bool {
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
	cfg := WalkForwardConfig{TrainSize: 200, TestSize: 50, Embargo: 5, NTrees: 3, MaxDepth: 3, MinSamples: 5}
	_, err := WalkForwardCV(samples, cfg, nil)
	if err == nil {
		t.Error("expected error for insufficient samples, got nil")
	}
}

func TestWalkForwardCVInvalidConfig(t *testing.T) {
	t.Parallel()
	samples := makeSamples(100)
	cfg := WalkForwardConfig{TrainSize: 0, TestSize: 50}
	if _, err := WalkForwardCV(samples, cfg, nil); err == nil {
		t.Error("expected error for TrainSize=0")
	}
	cfg2 := WalkForwardConfig{TrainSize: 50, TestSize: 0}
	if _, err := WalkForwardCV(samples, cfg2, nil); err == nil {
		t.Error("expected error for TestSize=0")
	}
}

func TestMergeForests(t *testing.T) {
	t.Parallel()
	samples := makeSamples(600)
	cfg := WalkForwardConfig{
		TrainSize:  200,
		TestSize:   50,
		Embargo:    5,
		NTrees:     5,
		MaxDepth:   3,
		MinSamples: 5,
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
	for _, r := range results {
		if r.Forest != nil {
			totalTrees += len(r.Forest.Trees)
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
	if got := MergeForests([]WalkForwardResult{{Forest: nil}}); got != nil {
		t.Errorf("MergeForests with nil forests = %v, want nil", got)
	}
}

func TestBestFold(t *testing.T) {
	t.Parallel()
	results := []WalkForwardResult{
		{Fold: 0, Metrics: Metrics{Sortino: 0.5}},
		{Fold: 1, Metrics: Metrics{Sortino: 1.5}},
		{Fold: 2, Metrics: Metrics{Sortino: 1.0}},
	}
	best := BestFold(results)
	if best == nil || best.Fold != 1 {
		t.Errorf("BestFold = fold %v, want fold 1", best)
	}
	if got := BestFold(nil); got != nil {
		t.Errorf("BestFold(nil) = %v, want nil", got)
	}
}
