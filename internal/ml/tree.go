package ml

import (
	"math"
	"slices"
)

const (
	// thresholdMidpointDivisor divides the sum of two adjacent unique values to get a midpoint.
	thresholdMidpointDivisor = 2.0
	// maxThresholdCandidates caps the number of candidate split thresholds per feature.
	maxThresholdCandidates = 32
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
	return &Tree{
		root:       nil,
		maxDepth:   maxDepth,
		minSamples: minSamples,
	}
}

// Fit trains the tree on samples, using only the feature indices in featureMask.
func (t *Tree) Fit(samples []Sample, featureMask []int) {
	t.root = buildNode(samples, featureMask, t.maxDepth, t.minSamples)
}

// Predict returns the regression prediction for a single feature vector.
func (t *Tree) Predict(features [featureCount]float64) float64 {
	return traverse(t.root, features)
}

func traverse(currentNode *node, features [featureCount]float64) float64 {
	if currentNode.isLeaf {
		return currentNode.prediction
	}

	if features[currentNode.featureIdx] <= currentNode.threshold {
		return traverse(currentNode.left, features)
	}

	return traverse(currentNode.right, features)
}

func buildNode(samples []Sample, mask []int, depth, minSamples int) *node {
	pred := meanLabel(samples)

	if depth == 0 || len(samples) < minSamples {
		return &node{
			featureIdx: 0,
			threshold:  0,
			left:       nil,
			right:      nil,
			isLeaf:     true,
			prediction: pred,
		}
	}

	bestFeat, bestThresh, bestGain := -1, 0.0, -math.MaxFloat64
	parentVar := variance(samples)

	for _, featIdx := range mask {
		thresholds := uniqueThresholds(samples, featIdx)

		for _, thresh := range thresholds {
			left, right := split(samples, featIdx, thresh)
			if len(left) == 0 || len(right) == 0 {
				continue
			}

			gain := parentVar - weightedVariance(left, right)
			if gain > bestGain {
				bestGain = gain
				bestFeat = featIdx
				bestThresh = thresh
			}
		}
	}

	if bestFeat == -1 || bestGain <= 0 {
		return &node{
			featureIdx: 0,
			threshold:  0,
			left:       nil,
			right:      nil,
			isLeaf:     true,
			prediction: pred,
		}
	}

	left, right := split(samples, bestFeat, bestThresh)

	return &node{
		featureIdx: bestFeat,
		threshold:  bestThresh,
		left:       buildNode(left, mask, depth-1, minSamples),
		right:      buildNode(right, mask, depth-1, minSamples),
		prediction: 0,
		isLeaf:     false,
	}
}

func meanLabel(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}

	sum := 0.0
	for idx := range samples {
		sum += samples[idx].Label
	}

	return sum / float64(len(samples))
}

func variance(samples []Sample) float64 {
	if len(samples) == 0 {
		return 0
	}

	mean := meanLabel(samples)
	varSum := 0.0

	for idx := range samples {
		diff := samples[idx].Label - mean
		varSum += diff * diff
	}

	return varSum / float64(len(samples))
}

func weightedVariance(left, right []Sample) float64 {
	total := float64(len(left) + len(right))

	return float64(len(left))/total*variance(left) + float64(len(right))/total*variance(right)
}

func split(samples []Sample, fi int, threshold float64) ([]Sample, []Sample) {
	var left, right []Sample

	for idx := range samples {
		if samples[idx].Features[fi] <= threshold {
			left = append(left, samples[idx])
		} else {
			right = append(right, samples[idx])
		}
	}

	return left, right
}

// uniqueThresholds returns candidate split points for feature fi: midpoints
// between consecutive sorted unique values (capped at 32 candidates).
func uniqueThresholds(samples []Sample, fi int) []float64 {
	vals := make([]float64, len(samples))
	for idx := range samples {
		vals[idx] = samples[idx].Features[fi]
	}

	slices.Sort(vals)

	seen := make(map[float64]bool)

	var unique []float64

	for _, val := range vals {
		if !seen[val] {
			seen[val] = true
			unique = append(unique, val)
		}
	}

	step := 1
	if len(unique)-1 > maxThresholdCandidates {
		step = (len(unique) - 1) / maxThresholdCandidates
	}

	var thresholds []float64

	for i := 0; i < len(unique)-1; i += step {
		thresholds = append(thresholds, (unique[i]+unique[i+1])/thresholdMidpointDivisor)
	}

	return thresholds
}
