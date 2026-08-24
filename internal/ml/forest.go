package ml

import (
	"math"

	"github.com/tgragnato/orbiter/internal/portfolio/features"
)

const (
	// minReturnsForSortino is the minimum number of returns required to compute Sortino.
	minReturnsForSortino = 2
	// maxSortinoRatio is the capped Sortino ratio when there is no downside deviation.
	maxSortinoRatio = 10.0
	// tradingDaysPerYear is the annualisation factor for daily return series.
	tradingDaysPerYear = 252
	// lcgMultiplier is the multiplier constant for the linear congruential generator.
	lcgMultiplier = 6364136223846793005
	// lcgIncrement is the increment constant for the linear congruential generator.
	lcgIncrement = 1442695040888963407
)

// Forest is a Random Forest regression ensemble. Each tree is trained on a
// bootstrap sample of the training data and a random subset of features.
type Forest struct {
	Trees      []*Tree
	nFeatures  int
	maxDepth   int
	minSamples int
}

// NewForest creates an untrained Forest. featuresPerSplit is the number of
// candidate features sampled at each split (m_try); 0 defaults to 12 (~40% of
// featureCount). The value is clamped to [1, featureCount].
func NewForest(nTrees, maxDepth, minSamples, featuresPerSplit int) *Forest {
	if featuresPerSplit <= 0 {
		featuresPerSplit = 12
	}

	if featuresPerSplit > featureCount {
		featuresPerSplit = featureCount
	}

	return &Forest{
		Trees:      make([]*Tree, 0, nTrees),
		nFeatures:  featuresPerSplit,
		maxDepth:   maxDepth,
		minSamples: minSamples,
	}
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

// Fit trains all trees on samples using bootstrap aggregation with random
// feature subsets. src provides deterministic pseudo-randomness; each tree i
// uses seed i so results are reproducible.
func (f *Forest) Fit(samples []Sample, nTrees int) {
	f.Trees = make([]*Tree, 0, nTrees)

	for i := range nTrees {
		rng := newLCG(uint64(i + 1))
		boot := bootstrap(samples, rng)
		mask := randomFeatureMask(featureCount, f.nFeatures, rng)
		treeNode := newTree(f.maxDepth, f.minSamples)
		treeNode.Fit(boot, mask)
		f.Trees = append(f.Trees, treeNode)
	}
}

// Predict returns the average prediction across all trees.
func (f *Forest) Predict(feat [featureCount]float64) float64 {
	if len(f.Trees) == 0 {
		return 0
	}

	sum := 0.0
	for _, treeItem := range f.Trees {
		sum += treeItem.Predict(feat)
	}

	return sum / float64(len(f.Trees))
}

// Metrics holds cross-validation metrics for a single walk-forward fold.
type Metrics struct {
	Fold    int
	MSE     float64
	MAE     float64
	Sortino float64
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

// MSE computes mean-squared error between predictions and labels.
func MSE(preds, labels []float64) float64 {
	if len(preds) != len(labels) || len(preds) == 0 {
		return 0
	}

	sum := 0.0

	for i := range preds {
		diff := preds[i] - labels[i]
		sum += diff * diff
	}

	return sum / float64(len(preds))
}

// Sortino computes an annualised Sortino ratio from a return series (assumes
// daily observations, 252 trading days/year). MAR is 0: only negative returns
// contribute to downside deviation, so positive volatility is not penalised.
func Sortino(returns []float64) float64 {
	if len(returns) < minReturnsForSortino {
		return 0
	}

	mean := features.Mean(returns)

	// Downside deviation: RMS of min(0, r_t) over all observations.
	sumSq := 0.0

	for _, ret := range returns {
		if ret < 0 {
			sumSq += ret * ret
		}
	}

	downsideDev := math.Sqrt(sumSq / float64(len(returns)))

	if downsideDev == 0 {
		if mean > 0 {
			return maxSortinoRatio
		}

		return 0
	}

	return mean / downsideDev * math.Sqrt(tradingDaysPerYear)
}

// --- internal helpers ---

// lcg is a simple linear congruential generator for reproducible shuffling.
type lcg struct{ state uint64 }

func newLCG(seed uint64) *lcg { return &lcg{state: seed} }

func (r *lcg) next() uint64 {
	r.state = r.state*lcgMultiplier + lcgIncrement

	return r.state
}

func (r *lcg) intn(numCandidates int) int {
	if numCandidates <= 0 {
		return 0
	}

	return int(r.next() % uint64(numCandidates)) // #nosec G115
}

func bootstrap(samples []Sample, rng *lcg) []Sample {
	numSamples := len(samples)
	out := make([]Sample, numSamples)

	for i := range out {
		out[i] = samples[rng.intn(numSamples)]
	}

	return out
}

func randomFeatureMask(total, numFeatures int, rng *lcg) []int {
	if numFeatures > total {
		numFeatures = total
	}

	indices := make([]int, total)
	for i := range indices {
		indices[i] = i
	}

	// Fisher-Yates partial shuffle for first numFeatures elements.
	for i := range numFeatures {
		j := i + rng.intn(total-i)
		indices[i], indices[j] = indices[j], indices[i]
	}

	return indices[:numFeatures]
}
