//nolint:testpackage // accesses unexported symbols: newTree, meanLabel, variance, split, newLCG
package ml

import (
	"math"
	"testing"
)

func makeSamples(numSamples int) []Sample {
	samples := make([]Sample, numSamples)
	rng := newLCG(42)

	for idx := range samples {
		val := float64(rng.next()%200) / 100.0 // [0, 2)

		for fi := range featureCount {
			samples[idx].Features[fi] = float64(rng.next()%1000) / 500.0 // [-1, 1) normalised later
		}

		samples[idx].Features[FeatRSI] = val - 1.0 // [-1, 1)
		// Label: positive when FeatRSI > 0 (linearly separable)
		samples[idx].Label = samples[idx].Features[FeatRSI] * 0.1
	}

	return samples
}

func TestTreeFitPredict(t *testing.T) {
	t.Parallel()

	samples := makeSamples(200)
	allFeatures := make([]int, featureCount)

	for i := range allFeatures {
		allFeatures[i] = i
	}

	testTree := newTree(5, 2)
	testTree.Fit(samples, allFeatures)

	// Tree should predict something in a reasonable range.
	var feat [featureCount]float64

	feat[FeatRSI] = 0.5
	pred := testTree.Predict(feat)

	if math.IsNaN(pred) || math.IsInf(pred, 0) {
		t.Errorf("Predict returned invalid value: %f", pred)
	}
}

func TestTreeEmptySamples(t *testing.T) {
	t.Parallel()

	testTree := newTree(5, 2)
	testTree.Fit([]Sample{}, []int{0})
	pred := testTree.Predict([featureCount]float64{})

	if math.IsNaN(pred) {
		t.Error("predict on empty-fit tree should return 0, not NaN")
	}
}

func TestMeanLabel(t *testing.T) {
	t.Parallel()

	samples := []Sample{
		{Features: [featureCount]float64{}, Label: 1},
		{Features: [featureCount]float64{}, Label: 2},
		{Features: [featureCount]float64{}, Label: 3},
	}

	if got := meanLabel(samples); math.Abs(got-2.0) > 1e-9 {
		t.Errorf("meanLabel = %f, want 2.0", got)
	}

	if got := meanLabel(nil); got != 0 {
		t.Errorf("meanLabel(nil) = %f, want 0", got)
	}
}

func TestVariance(t *testing.T) {
	t.Parallel()

	samples := []Sample{
		{Features: [featureCount]float64{}, Label: 1},
		{Features: [featureCount]float64{}, Label: 2},
		{Features: [featureCount]float64{}, Label: 3},
	}
	varResult := variance(samples)

	if varResult <= 0 {
		t.Errorf("variance = %f, want > 0", varResult)
	}

	if got := variance(nil); got != 0 {
		t.Errorf("variance(nil) = %f, want 0", got)
	}
}

func TestUniqueThresholds(t *testing.T) {
	t.Parallel()

	samples := []Sample{
		{Features: [featureCount]float64{0: 1.0}, Label: 0},
		{Features: [featureCount]float64{0: 2.0}, Label: 0},
		{Features: [featureCount]float64{0: 3.0}, Label: 0},
	}
	thresholds := uniqueThresholds(samples, 0)

	if len(thresholds) == 0 {
		t.Error("expected thresholds, got none")
	}

	for _, th := range thresholds {
		if math.IsNaN(th) {
			t.Error("NaN threshold")
		}
	}
}

func TestSplit(t *testing.T) {
	t.Parallel()

	samples := []Sample{
		{Features: [featureCount]float64{0: 1.0}, Label: 1},
		{Features: [featureCount]float64{0: 3.0}, Label: -1},
	}
	left, right := split(samples, 0, 2.0)

	if len(left) != 1 || len(right) != 1 {
		t.Errorf("split: left=%d right=%d, want 1 1", len(left), len(right))
	}
}
