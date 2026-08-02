package ml

import (
	"math"

	"github.com/tgragnato/orbiter/internal/portfolio/features"
)

// Forest is a Random Forest regression ensemble. Each tree is trained on a
// bootstrap sample of the training data and a random subset of features.
type Forest struct {
	Trees      []*Tree
	nFeatures  int
	maxDepth   int
	minSamples int
}

// NewForest creates an untrained Forest.
func NewForest(nTrees, maxDepth, minSamples int) *Forest {
	return &Forest{
		Trees:      make([]*Tree, 0, nTrees),
		nFeatures:  int(math.Round(math.Sqrt(float64(featureCount)))),
		maxDepth:   maxDepth,
		minSamples: minSamples,
	}
}

// Fit trains all trees on samples using bootstrap aggregation with random
// feature subsets. src provides deterministic pseudo-randomness; each tree i
// uses seed i so results are reproducible.
func (f *Forest) Fit(samples []Sample, nTrees int) {
	f.Trees = make([]*Tree, 0, nTrees)
	for i := 0; i < nTrees; i++ {
		rng := newLCG(uint64(i + 1))
		boot := bootstrap(samples, rng)
		mask := randomFeatureMask(featureCount, f.nFeatures, rng)
		t := newTree(f.maxDepth, f.minSamples)
		t.Fit(boot, mask)
		f.Trees = append(f.Trees, t)
	}
}

// Predict returns the average prediction across all trees.
func (f *Forest) Predict(feat [featureCount]float64) float64 {
	if len(f.Trees) == 0 {
		return 0
	}
	sum := 0.0
	for _, t := range f.Trees {
		sum += t.Predict(feat)
	}
	return sum / float64(len(f.Trees))
}

// ConvictionScore converts the raw forest prediction (expected log-return) to
// a bounded conviction score in [-1,+1] using a soft tanh transform scaled by
// the cross-validated standard deviation of predictions.
func (f *Forest) ConvictionScore(feat [featureCount]float64, scale float64) float64 {
	if scale <= 0 {
		scale = 0.01
	}
	raw := f.Predict(feat)
	return math.Tanh(raw / scale)
}

// Metrics holds cross-validation metrics for a single walk-forward fold.
type Metrics struct {
	Fold  int
	MSE   float64
	MAE   float64
	Sharpe float64
}

// MSE computes mean-squared error between predictions and labels.
func MSE(preds, labels []float64) float64 {
	if len(preds) != len(labels) || len(preds) == 0 {
		return 0
	}
	sum := 0.0
	for i := range preds {
		d := preds[i] - labels[i]
		sum += d * d
	}
	return sum / float64(len(preds))
}

// MAE computes mean-absolute error.
func MAE(preds, labels []float64) float64 {
	if len(preds) != len(labels) || len(preds) == 0 {
		return 0
	}
	sum := 0.0
	for i := range preds {
		sum += math.Abs(preds[i] - labels[i])
	}
	return sum / float64(len(preds))
}

// Sharpe computes an annualised Sharpe ratio from a return series (assumes
// daily observations, 252 trading days/year).
func Sharpe(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	mean := features.Mean(returns)
	std := features.StdDev(returns, mean)
	if std == 0 {
		return 0
	}
	return mean / std * math.Sqrt(252)
}

// --- internal helpers ---

// lcg is a simple linear congruential generator for reproducible shuffling.
type lcg struct{ state uint64 }

func newLCG(seed uint64) *lcg { return &lcg{state: seed} }

func (r *lcg) next() uint64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	return r.state
}

func (r *lcg) intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.next() % uint64(n))
}

func bootstrap(samples []Sample, rng *lcg) []Sample {
	n := len(samples)
	out := make([]Sample, n)
	for i := range out {
		out[i] = samples[rng.intn(n)]
	}
	return out
}

func randomFeatureMask(total, k int, rng *lcg) []int {
	if k > total {
		k = total
	}
	indices := make([]int, total)
	for i := range indices {
		indices[i] = i
	}
	// Fisher-Yates partial shuffle for first k elements.
	for i := 0; i < k; i++ {
		j := i + rng.intn(total-i)
		indices[i], indices[j] = indices[j], indices[i]
	}
	return indices[:k]
}
