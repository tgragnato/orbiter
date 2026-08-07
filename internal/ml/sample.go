// Package ml implements a pure-Go Random Forest ensemble trained via
// purged walk-forward cross-validation on tabular financial feature vectors.
package ml

const (
	// Feature indices — keep in sync with featureCount.
	// Indices 0–12: batch indicators computed by go-talib over EOD arrays.
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
	// Indices 13–22: conviction scores from ScoredStrategy implementations,
	// computed incrementally (no lookahead) using the same streaming logic
	// that would drive live trading decisions.
	FeatScoreDoji     = 13
	FeatScoreEngulf   = 14
	FeatScoreHarami   = 15
	FeatScoreHA       = 16
	FeatScoreLowCand  = 17
	FeatScoreRSI      = 18
	FeatScoreRSIADX   = 19
	FeatScoreScalper  = 20
	FeatScoreSMA10    = 21
	FeatScoreStochRSI = 22
	// Indices 23–25: incremental indicators from pkg/indicator/stoch and pkg/indicator/round.
	FeatStochK    = 23 // Fast Stochastic %K / 100 → [0,1]
	FeatStochD    = 24 // Fast Stochastic %D / 100 → [0,1]
	FeatRoundWeak = 25 // (close − lowerWeak) / (upperWeak − lowerWeak) → [0,1]
	featureCount  = 26
)

// Sample is one training or inference data point.
// Label is the next-period return (regression target); it is ignored during inference.
type Sample struct {
	Features [featureCount]float64
	Label    float64 // next-period log-return
}

