package ml

import (
	"math"
	"sort"
)

// node is a single node in a CART regression tree.
type node struct {
	featureIdx int
	threshold  float64
	left       *node
	right      *node
	prediction float64 // leaf value: mean label of training samples at this node
	isLeaf     bool
}

// Tree is a CART regression decision tree.
type Tree struct {
	root       *node
	maxDepth   int
	minSamples int
}

// newTree returns an untrained Tree with the given hyper-parameters.
func newTree(maxDepth, minSamples int) *Tree {
	return &Tree{maxDepth: maxDepth, minSamples: minSamples}
}

// Fit trains the tree on samples, using only the feature indices in featureMask.
func (t *Tree) Fit(samples []Sample, featureMask []int) {
	t.root = buildNode(samples, featureMask, t.maxDepth, t.minSamples)
}

// Predict returns the regression prediction for a single feature vector.
func (t *Tree) Predict(features [featureCount]float64) float64 {
	return traverse(t.root, features)
}

func traverse(n *node, features [featureCount]float64) float64 {
	if n.isLeaf {
		return n.prediction
	}
	if features[n.featureIdx] <= n.threshold {
		return traverse(n.left, features)
	}
	return traverse(n.right, features)
}

func buildNode(samples []Sample, mask []int, depth, minSamples int) *node {
	pred := meanLabel(samples)

	if depth == 0 || len(samples) < minSamples {
		return &node{isLeaf: true, prediction: pred}
	}

	bestFeat, bestThresh, bestGain := -1, 0.0, -math.MaxFloat64
	parentVar := variance(samples)

	for _, fi := range mask {
		thresholds := uniqueThresholds(samples, fi)
		for _, t := range thresholds {
			left, right := split(samples, fi, t)
			if len(left) == 0 || len(right) == 0 {
				continue
			}
			gain := parentVar - weightedVariance(left, right)
			if gain > bestGain {
				bestGain = gain
				bestFeat = fi
				bestThresh = t
			}
		}
	}

	if bestFeat == -1 || bestGain <= 0 {
		return &node{isLeaf: true, prediction: pred}
	}

	left, right := split(samples, bestFeat, bestThresh)
	return &node{
		featureIdx: bestFeat,
		threshold:  bestThresh,
		left:       buildNode(left, mask, depth-1, minSamples),
		right:       buildNode(right, mask, depth-1, minSamples),
	}
}

func meanLabel(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range samples {
		sum += s.Label
	}
	return sum / float64(len(samples))
}

func variance(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}
	mean := meanLabel(samples)
	v := 0.0
	for _, s := range samples {
		d := s.Label - mean
		v += d * d
	}
	return v / float64(len(samples))
}

func weightedVariance(left, right []Sample) float64 {
	n := float64(len(left) + len(right))
	return float64(len(left))/n*variance(left) + float64(len(right))/n*variance(right)
}

func split(samples []Sample, fi int, threshold float64) (left, right []Sample) {
	for _, s := range samples {
		if s.Features[fi] <= threshold {
			left = append(left, s)
		} else {
			right = append(right, s)
		}
	}
	return
}

// uniqueThresholds returns candidate split points for feature fi: midpoints
// between consecutive sorted unique values (capped at 32 candidates).
func uniqueThresholds(samples []Sample, fi int) []float64 {
	vals := make([]float64, len(samples))
	for i, s := range samples {
		vals[i] = s.Features[fi]
	}
	sort.Float64s(vals)

	seen := make(map[float64]bool)
	var unique []float64
	for _, v := range vals {
		if !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}

	const maxCandidates = 32
	step := 1
	if len(unique)-1 > maxCandidates {
		step = (len(unique) - 1) / maxCandidates
	}

	var thresholds []float64
	for i := 0; i < len(unique)-1; i += step {
		thresholds = append(thresholds, (unique[i]+unique[i+1])/2.0)
	}
	return thresholds
}
