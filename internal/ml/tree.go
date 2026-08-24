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

// featurePair associates a feature value with its original sample index.
type featurePair struct {
	val float64
	idx int
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
	numSamples := len(samples)
	if numSamples == 0 {
		return
	}

	maxFeatIdx := 0
	for _, f := range featureMask {
		if f > maxFeatIdx {
			maxFeatIdx = f
		}
	}

	preSorted := make([][]featurePair, maxFeatIdx+1)
	for _, featIdx := range featureMask {
		pairs := make([]featurePair, numSamples)
		for i := range samples {
			pairs[i] = featurePair{
				val: samples[i].Features[featIdx],
				idx: i,
			}
		}

		slices.SortFunc(pairs, compareFeaturePairs)

		preSorted[featIdx] = pairs
	}

	activeMask := make([]bool, numSamples)
	for i := range activeMask {
		activeMask[i] = true
	}

	t.root = buildNode(samples, featureMask, t.maxDepth, t.minSamples, preSorted, activeMask, numSamples)
}

// Predict returns the regression prediction for a single feature vector.
func (t *Tree) Predict(features [featureCount]float64) float64 {
	return traverse(t.root, features)
}

// traverse recursively navigates the decision tree from currentNode based on
// feature values until it reaches a leaf, returning the predicted value.
func traverse(currentNode *node, features [featureCount]float64) float64 {
	if currentNode == nil {
		return 0.0
	}

	if currentNode.isLeaf {
		return currentNode.prediction
	}

	if features[currentNode.featureIdx] <= currentNode.threshold {
		return traverse(currentNode.left, features)
	}

	return traverse(currentNode.right, features)
}

// buildNode recursively constructs a tree node by finding the feature split point
// that yields the maximum variance reduction for the currently active samples.
func buildNode( //nolint:cyclop,funlen // satisfy gocritic
	samples []Sample,
	mask []int,
	depth, minSamples int,
	preSorted [][]featurePair,
	activeMask []bool,
	activeCount int,
) *node {
	// Calculate statistics for active samples
	pred, parentVar, totalSum, totalSqSum := calcStatsActive(samples, activeMask, activeCount)

	if depth == 0 || activeCount < minSamples {
		return &node{
			featureIdx: 0,
			threshold:  0.0,
			left:       nil,
			right:      nil,
			prediction: pred,
			isLeaf:     true,
		}
	}

	// Find the best feature split that yields maximum variance reduction
	bestFeat, bestThresh, bestGain := -1, 0.0, -math.MaxFloat64
	nTotal := float64(activeCount)

	for _, featIdx := range mask {
		sortedPairs := preSorted[featIdx]

		var leftSum, leftSqSum float64

		var countLeft int

		var prevVal float64

		var hasPrev bool

		step := 1
		if activeCount > maxThresholdCandidates {
			step = activeCount / maxThresholdCandidates
		}

		for _, pair := range sortedPairs {
			if !activeMask[pair.idx] {
				continue
			}

			currVal := pair.val

			if hasPrev && currVal != prevVal && countLeft%step == 0 {
				nLeft := float64(countLeft)
				nRight := nTotal - nLeft

				rightSum := totalSum - leftSum
				rightSqSum := totalSqSum - leftSqSum

				leftVar := (leftSqSum / nLeft) - (leftSum/nLeft)*(leftSum/nLeft)
				rightVar := (rightSqSum / nRight) - (rightSum/nRight)*(rightSum/nRight)

				weightedVar := (nLeft/nTotal)*leftVar + (nRight/nTotal)*rightVar
				gain := parentVar - weightedVar

				if gain > bestGain {
					bestGain = gain
					bestFeat = featIdx
					bestThresh = (prevVal + currVal) / thresholdMidpointDivisor
				}
			}

			v := samples[pair.idx].Label
			leftSum += v
			leftSqSum += v * v
			countLeft++

			prevVal = currVal
			hasPrev = true
		}
	}

	if bestFeat == -1 || bestGain <= 0 {
		return &node{
			featureIdx: 0,
			threshold:  0.0,
			left:       nil,
			right:      nil,
			prediction: pred,
			isLeaf:     true,
		}
	}

	leftMask := make([]bool, len(samples))
	rightMask := make([]bool, len(samples))

	var leftCount, rightCount int

	for active, mask := range activeMask {
		if !mask {
			continue
		}

		if samples[active].Features[bestFeat] <= bestThresh {
			leftMask[active] = true
			leftCount++
		} else {
			rightMask[active] = true
			rightCount++
		}
	}

	return &node{
		featureIdx: bestFeat,
		threshold:  bestThresh,
		left:       buildNode(samples, mask, depth-1, minSamples, preSorted, leftMask, leftCount),
		right:      buildNode(samples, mask, depth-1, minSamples, preSorted, rightMask, rightCount),
		prediction: 0.0, // Not used for non-leaf nodes
		isLeaf:     false,
	}
}

// calcStatsActive computes the mean, variance, sum, and squared sum of labels
// for samples marked as true in activeMask.
func calcStatsActive(samples []Sample, activeMask []bool, activeCount int) (
	mean, variance, sum, sqSum float64) { //nolint:nonamedreturns // satisfy gocritic
	if activeCount == 0 {
		return 0, 0, 0, 0
	}

	for i, active := range activeMask {
		if !active {
			continue
		}

		v := samples[i].Label
		sum += v
		sqSum += v * v
	}

	n := float64(activeCount)
	mean = sum / n
	variance = (sqSum / n) - (mean * mean)

	if variance < 0 {
		variance = 0
	}

	return mean, variance, sum, sqSum
}

// compareFeaturePairs provides an ascending order comparison between two featurePair
// structs based on their feature values, compatible with slices.SortFunc.
func compareFeaturePairs(a, b featurePair) int {
	if a.val < b.val {
		return -1
	}

	if a.val > b.val {
		return 1
	}

	return 0
}

// split reorders the Samples via swap in O(N) without allocating new memory.
func split(samples []Sample, fi int, threshold float64) ([]Sample, []Sample) {
	left := 0
	right := len(samples) - 1

	for left <= right {
		if samples[left].Features[fi] <= threshold {
			left++
		} else {
			samples[left], samples[right] = samples[right], samples[left]
			right--
		}
	}

	return samples[:left], samples[left:]
}

// uniqueThresholds returns candidate split points for feature fi: midpoints
// between consecutive sorted unique values (capped at 32 candidates).
func uniqueThresholds(samples []Sample, fi int) []float64 {
	vals := make([]float64, len(samples))
	for idx := range samples {
		vals[idx] = samples[idx].Features[fi]
	}

	slices.Sort(vals)

	uniqueLen := 0

	for i := range vals {
		if i == 0 || vals[i] != vals[i-1] {
			vals[uniqueLen] = vals[i]
			uniqueLen++
		}
	}

	unique := vals[:uniqueLen]

	step := 1
	if len(unique)-1 > maxThresholdCandidates {
		step = (len(unique) - 1) / maxThresholdCandidates
	}

	thresholds := make([]float64, 0, maxThresholdCandidates)
	for i := 0; i < len(unique)-1; i += step {
		thresholds = append(thresholds, (unique[i]+unique[i+1])/thresholdMidpointDivisor)
	}

	return thresholds
}
