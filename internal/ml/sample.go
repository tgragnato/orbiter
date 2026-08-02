// Package ml implements a pure-Go Random Forest ensemble trained via
// purged walk-forward cross-validation on tabular financial feature vectors.
package ml

const (
	// Feature indices — keep in sync with featureCount.
	FeatRSI        = 0
	FeatStochRSI   = 1
	FeatRSIADX     = 2
	FeatSMA10      = 3
	FeatLowCandle  = 4
	FeatEngulfing  = 5
	FeatHarami     = 6
	FeatHA         = 7
	FeatScalper    = 8
	FeatReturn1    = 9
	FeatReturn5    = 10
	FeatReturn20   = 11
	FeatZScore20   = 12
	featureCount   = 13
)

// Sample is one training or inference data point.
// Label is the next-period return (regression target); it is ignored during inference.
type Sample struct {
	Features [featureCount]float64
	Label    float64 // next-period log-return
}

// FeatureCount returns the fixed dimension of the feature vector.
func FeatureCount() int { return featureCount }
