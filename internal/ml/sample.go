// Package ml implements a pure-Go Random Forest ensemble trained via
// purged walk-forward cross-validation on tabular financial feature vectors.
package ml

// Feature index constants — keep in sync with featureCount.
// Indices 0–12: batch indicators (go-talib over EOD arrays).
// Indices 13–22: conviction scores from ScoredStrategy implementations.
// Indices 23–25: incremental indicators from pkg/indicator/stoch and pkg/indicator/round.
// Indices 26–29: relative strength vs the portfolio benchmark and medium-term momentum.
// Indices 30–32: primary trend regime (SMA50 / SMA200 distance and SMA50 slope).
// Indices 33–35: volatility regime (normalised ATR and short/medium volatility ratio).
const (
	FeatRSI           = 0
	FeatStochRSI      = 1
	FeatRSIADX        = 2
	FeatSMA10         = 3
	FeatLowCandle     = 4
	FeatEngulfing     = 5
	FeatHarami        = 6
	FeatHA            = 7
	FeatScalper       = 8
	FeatReturn1       = 9
	FeatReturn5       = 10
	FeatReturn20      = 11
	FeatZScore20      = 12
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
	FeatStochK        = 23 // Fast Stochastic %K / 100 → [0,1]
	FeatStochD        = 24 // Fast Stochastic %D / 100 → [0,1]
	FeatRoundWeak     = 25 // (close − lowerWeak) / (upperWeak − lowerWeak) → [0,1]
	FeatRelReturn20   = 26 // 20-day asset log-return minus 20-day portfolio benchmark log-return
	FeatRelReturn60   = 27 // 60-day asset log-return minus 60-day portfolio benchmark log-return
	FeatReturn60      = 28 // log(close[i] / close[i−60]) — ~3-month momentum
	FeatReturn120     = 29 // log(close[i] / close[i−120]) — ~6-month momentum
	FeatSMA50         = 30 // (close − SMA50) / SMA50 — distance from the intermediate trend
	FeatSMA200        = 31 // (close − SMA200) / SMA200 — distance from the primary trend
	FeatSMA50Slope    = 32 // (SMA50[i] − SMA50[i−10]) / SMA50[i−10] — primary trend acceleration
	FeatNormATR14     = 33 // ATR(14) / close — short-horizon volatility, comparable across assets
	FeatNormATR50     = 34 // ATR(50) / close — medium-horizon volatility baseline
	FeatVolRatio      = 35 // σ20 / σ60 of daily log-returns — < 1 marks volatility compression
	featureCount      = 36
)

// Sample is one training or inference data point.
// Label is the next-period return (regression target); it is ignored during inference.
type Sample struct {
	Features [featureCount]float64
	Label    float64 // next-period log-return
}
