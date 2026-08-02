// Package featurizer converts raw EOD candle history for a portfolio into
// ml.Sample vectors ready for walk-forward training. It sits above both the
// features and ml packages to avoid import cycles.
package featurizer

import (
	"context"
	"math"
	"time"

	talib "github.com/markcheno/go-talib"
	"github.com/tgragnato/orbiter/internal/ml"
	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/portfolio/data"
	"github.com/tgragnato/orbiter/internal/portfolio/features"
)

const (
	historyYears = 3  // years of EOD history requested per symbol
	warmupBars   = 40 // leading bars consumed for indicator convergence and discarded
	forwardDays  = 1  // label horizon: 1-trading-day forward log-return
)

// ExtractMLSamples fetches historyYears of EOD candles for every active
// holding (Quantity > 0) and converts them into ml.Sample vectors ready for
// walk-forward training. Symbols whose fetch fails or whose history is too
// short are silently skipped so a partial portfolio still yields samples.
func ExtractMLSamples(ctx context.Context, store portfolio.HoldingsStore, provider data.DataProvider) ([]ml.Sample, error) {
	holdings, err := store.ListHoldings(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	from := now.AddDate(-historyYears, 0, 0)

	seen := make(map[string]bool)
	var all []ml.Sample
	for _, h := range holdings {
		if h.Quantity <= 0 || seen[h.Symbol] {
			continue
		}
		seen[h.Symbol] = true

		candles, err := provider.GetEOD(h.Symbol, from, now)
		if err != nil || len(candles) < warmupBars+forwardDays+1 {
			continue
		}
		all = append(all, samplesFromCandles(candles)...)
	}
	return all, nil
}

// samplesFromCandles converts a chronological EOD candle slice for one symbol
// into ml.Sample vectors. warmupBars leading bars are consumed by the
// indicators and discarded; the final bar is reserved as the label for the
// preceding row, so it is not itself turned into a sample.
func samplesFromCandles(candles []data.Candle) []ml.Sample {
	n := len(candles)
	opens  := make([]float64, n)
	highs  := make([]float64, n)
	lows   := make([]float64, n)
	closes := make([]float64, n)
	for i, c := range candles {
		opens[i]  = c.Open
		highs[i]  = c.High
		lows[i]   = c.Low
		closes[i] = c.AdjustedClose
		if closes[i] <= 0 {
			closes[i] = c.Close
		}
	}

	rsiSeries   := talib.Rsi(closes, 14)
	adxSeries   := talib.Adx(highs, lows, closes, 14)
	sma10Series := talib.Sma(closes, 10)
	stochK, _   := talib.StochRsi(closes, 14, 14, 3, talib.SMA)
	engulf      := engulfingSignals(opens, closes)
	harami      := haramiSignals(opens, closes)
	hammer      := hammerSignals(opens, highs, lows, closes)
	haSignals   := heikinAshiSignals(opens, highs, lows, closes)

	var samples []ml.Sample
	for i := warmupBars; i < n-forwardDays; i++ {
		c, cnext := closes[i], closes[i+forwardDays]
		if c <= 0 || cnext <= 0 {
			continue
		}

		var s ml.Sample
		s.Features[ml.FeatRSI]       = rsiSeries[i] / 100.0
		s.Features[ml.FeatStochRSI]  = stochK[i] / 100.0
		s.Features[ml.FeatRSIADX]    = (rsiSeries[i] / 100.0) * (adxSeries[i] / 100.0)
		s.Features[ml.FeatSMA10]     = relToSMA(c, sma10Series[i])
		s.Features[ml.FeatLowCandle] = hammer[i] / 100.0
		s.Features[ml.FeatEngulfing] = engulf[i] / 100.0
		s.Features[ml.FeatHarami]    = harami[i] / 100.0
		s.Features[ml.FeatHA]        = haSignals[i]
		s.Features[ml.FeatScalper]   = bodyRatio(opens[i], highs[i], lows[i], c)
		s.Features[ml.FeatReturn1]   = logRet(closes, i, 1)
		s.Features[ml.FeatReturn5]   = logRet(closes, i, 5)
		s.Features[ml.FeatReturn20]  = logRet(closes, i, 20)
		s.Features[ml.FeatZScore20]  = returnZScore(closes, i, 20)
		s.Label = math.Log(cnext / c)
		samples = append(samples, s)
	}
	return samples
}

// relToSMA returns (price - sma) / sma, the price deviation from its 10-day moving average.
func relToSMA(price, sma float64) float64 {
	if sma == 0 {
		return 0
	}
	return (price - sma) / sma
}

// logRet returns the log-return of closes[i] relative to closes[i-k].
func logRet(closes []float64, i, k int) float64 {
	if i < k || closes[i-k] <= 0 {
		return 0
	}
	return math.Log(closes[i] / closes[i-k])
}

// returnZScore computes the z-score of the current 1-day log-return against
// the rolling window of the preceding `window` 1-day log-returns.
// Uses only past data so there is no lookahead bias.
func returnZScore(closes []float64, i, window int) float64 {
	if i < window+1 {
		return 0
	}
	rets := make([]float64, window)
	for j := 0; j < window; j++ {
		idx := i - window + j
		if closes[idx] <= 0 || closes[idx+1] <= 0 {
			return 0
		}
		rets[j] = math.Log(closes[idx+1] / closes[idx])
	}
	mean := features.Mean(rets)
	std := features.StdDev(rets, mean)
	if std == 0 {
		return 0
	}
	curr := logRet(closes, i, 1)
	return (curr - mean) / std
}

// bodyRatio returns (close - open) / (high - low), a scale-free measure of
// candle body directionality used as the "scalper" signal.
func bodyRatio(open, high, low, close float64) float64 {
	rangeHL := high - low
	if rangeHL == 0 {
		return 0
	}
	return (close - open) / rangeHL
}

// hammerSignals returns +1 when bar i is a hammer (small body at top, lower
// shadow > 2× body, upper shadow < body), -1 for an inverted hammer, 0 otherwise.
func hammerSignals(opens, highs, lows, closes []float64) []float64 {
	n := len(closes)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		body := math.Abs(closes[i] - opens[i])
		lowerShadow := math.Min(opens[i], closes[i]) - lows[i]
		upperShadow := highs[i] - math.Max(opens[i], closes[i])
		if body == 0 {
			continue
		}
		if lowerShadow > 2*body && upperShadow < body {
			out[i] = 1
		} else if upperShadow > 2*body && lowerShadow < body {
			out[i] = -1
		}
	}
	return out
}

// engulfingSignals returns +1 for a bullish engulfing (bearish bar followed by
// a bullish bar whose body completely contains the prior body), -1 for bearish.
func engulfingSignals(opens, closes []float64) []float64 {
	n := len(closes)
	out := make([]float64, n)
	for i := 1; i < n; i++ {
		prevBody := closes[i-1] - opens[i-1]
		currBody := closes[i] - opens[i]
		if prevBody < 0 && currBody > 0 &&
			closes[i] >= opens[i-1] && opens[i] <= closes[i-1] {
			out[i] = 1
		} else if prevBody > 0 && currBody < 0 &&
			closes[i] <= opens[i-1] && opens[i] >= closes[i-1] {
			out[i] = -1
		}
	}
	return out
}

// haramiSignals returns +1 for a bullish harami (bearish bar followed by a
// small bullish bar whose body fits inside the prior body), -1 for bearish.
func haramiSignals(opens, closes []float64) []float64 {
	n := len(closes)
	out := make([]float64, n)
	for i := 1; i < n; i++ {
		prevLo := math.Min(opens[i-1], closes[i-1])
		prevHi := math.Max(opens[i-1], closes[i-1])
		currLo := math.Min(opens[i], closes[i])
		currHi := math.Max(opens[i], closes[i])
		prevBearish := closes[i-1] < opens[i-1]
		currBullish := closes[i] > opens[i]
		if prevBearish && currBullish && currLo >= prevLo && currHi <= prevHi {
			out[i] = 1
		} else if !prevBearish && !currBullish && currLo >= prevLo && currHi <= prevHi {
			out[i] = -1
		}
	}
	return out
}

// heikinAshiSignals computes (haClose - haOpen) / haOpen for each bar.
// The result is 0 for the first bar (no prior HA candle available).
func heikinAshiSignals(opens, highs, lows, closes []float64) []float64 {
	n := len(closes)
	signals := make([]float64, n)
	if n == 0 {
		return signals
	}
	haO := opens[0]
	haC := (opens[0] + highs[0] + lows[0] + closes[0]) / 4.0
	for i := 1; i < n; i++ {
		prevHaO, prevHaC := haO, haC
		haC = (opens[i] + highs[i] + lows[i] + closes[i]) / 4.0
		haO = (prevHaO + prevHaC) / 2.0
		if haO > 0 {
			signals[i] = (haC - haO) / haO
		}
	}
	return signals
}
