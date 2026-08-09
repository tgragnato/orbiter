// Package featurizer converts raw EOD candle history for a portfolio into
// ml.Sample vectors ready for walk-forward training. It sits above both the
// features and ml packages to avoid import cycles.
package featurizer

import (
	"context"
	"math"
	"time"

	talib "github.com/markcheno/go-talib"
	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/internal/ml"
	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/portfolio/data"
	"github.com/tgragnato/orbiter/internal/portfolio/features"
	"github.com/tgragnato/orbiter/pkg/indicator/round"
	"github.com/tgragnato/orbiter/pkg/indicator/stoch"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	stratha "github.com/tgragnato/orbiter/pkg/strategy/HeikinAshi"
	stratdoji "github.com/tgragnato/orbiter/pkg/strategy/doji"
	stratengulf "github.com/tgragnato/orbiter/pkg/strategy/engulfing"
	stratharami "github.com/tgragnato/orbiter/pkg/strategy/harami"
	stratlowcandle "github.com/tgragnato/orbiter/pkg/strategy/lowcandle"
	stratrsi "github.com/tgragnato/orbiter/pkg/strategy/rsi"
	stratrsiadx "github.com/tgragnato/orbiter/pkg/strategy/rsiadx"
	stratscalper "github.com/tgragnato/orbiter/pkg/strategy/scalper"
	stratsma10 "github.com/tgragnato/orbiter/pkg/strategy/sma10"
	stratstochrsi "github.com/tgragnato/orbiter/pkg/strategy/stochrsi"
)

const (
	historyYears = 8  // years of EOD history requested per symbol (~2016 bars, ~11 walk-forward folds with TrainSize=1250)
	warmupBars   = 40 // leading bars consumed for indicator convergence and discarded
	forwardDays  = 5  // label horizon: 5-trading-day forward log-return (improves SNR over 1-day)
)

// CurrentSamples returns the most recent feature vector for each TAA-eligible
// holding, including those with Quantity=0 (closed positions preserved in the
// portfolio). Zero-qty holdings are included so the TAA entry-signal path can
// compute conviction scores and suggest re-entry when the asset returns to trend.
// The Label field is populated but should be ignored — it is a current-bar
// feature vector, not a forward-return target.
func CurrentSamples(ctx context.Context, store portfolio.HoldingsStore, provider data.DataProvider) (map[string]ml.Sample, error) {
	holdings, err := store.ListHoldings(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	from := now.AddDate(-historyYears, 0, 0)

	result := make(map[string]ml.Sample)
	seen := make(map[string]bool)
	for _, h := range holdings {
		if !h.TAAEnabled || seen[h.Symbol] {
			continue
		}
		seen[h.Symbol] = true

		candles, err := provider.GetEOD(h.Symbol, from, now)
		if err != nil || len(candles) < warmupBars+forwardDays+1 {
			continue
		}
		samples := samplesFromCandles(h.Symbol, candles)
		if len(samples) == 0 {
			continue
		}
		result[h.Symbol] = samples[len(samples)-1]
	}
	return result, nil
}

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
		all = append(all, samplesFromCandles(h.Symbol, candles)...)
	}
	return all, nil
}

// candleToOHLC converts a data.Candle (float64 fields) to *ohlc.OHLC
// (decimal.Decimal fields) so strategy indicators can consume it.
// The resulting candle is treated as a closed EOD bar.
func candleToOHLC(c data.Candle, symbol string) *ohlc.OHLC {
	price := c.AdjustedClose
	if price <= 0 {
		price = c.Close
	}
	o := ohlc.New(symbol, c.Time, 24*time.Hour, false)
	o.Open = decimal.NewFromFloat(c.Open)
	o.High = decimal.NewFromFloat(c.High)
	o.Low = decimal.NewFromFloat(c.Low)
	o.Close = decimal.NewFromFloat(price)
	// EOD bars are always closed; ForceClose is required so pkg/indicator
	// implementations (rsi, adx, sma, stoch, stochrsi) don't silently drop
	// the bar when checking o.Closed().
	o.ForceClose()
	return o
}

// newScoredStrategies instantiates one of each ScoredStrategy implementation.
// All use 24 h as the candle duration (EOD data).
func newScoredStrategies(symbol string) []strategy.Strategy {
	dur := 24 * time.Hour
	return []strategy.Strategy{
		stratdoji.New(symbol),
		stratengulf.New(symbol, dur),
		stratharami.New(symbol, dur),
		stratha.New(symbol),
		stratlowcandle.New(symbol, dur),
		stratrsi.New(symbol, dur),
		stratrsiadx.New(symbol, dur),
		stratscalper.New(symbol),
		stratsma10.New(symbol, dur),
		stratstochrsi.New(symbol),
	}
}

// samplesFromCandles converts a chronological EOD candle slice for one symbol
// into ml.Sample vectors. warmupBars leading bars are consumed by the
// indicators and discarded; the final bar is reserved as the label for the
// preceding row, so it is not itself turned into a sample.
func samplesFromCandles(symbol string, candles []data.Candle) []ml.Sample {
	n := len(candles)
	opens := make([]float64, n)
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	for i, c := range candles {
		opens[i] = c.Open
		highs[i] = c.High
		lows[i] = c.Low
		closes[i] = c.AdjustedClose
		if closes[i] <= 0 {
			closes[i] = c.Close
		}
	}

	rsiSeries := talib.Rsi(closes, 14)
	adxSeries := talib.Adx(highs, lows, closes, 14)
	sma10Series := talib.Sma(closes, 10)
	stochK, _ := talib.StochRsi(closes, 14, 14, 3, talib.SMA)
	engulf := engulfingSignals(opens, closes)
	harami := haramiSignals(opens, closes)
	hammer := hammerSignals(opens, highs, lows, closes)
	haSignals := heikinAshiSignals(opens, highs, lows, closes)

	// Convert all candles to *ohlc.OHLC once for strategy consumption.
	ohlcSlice := make([]*ohlc.OHLC, n)
	for i, c := range candles {
		ohlcSlice[i] = candleToOHLC(c, symbol)
	}

	// Instantiate all ScoredStrategies and incremental indicators, then feed
	// the warm-up window. OnWarmUpCandle / Insert on every bar keeps state
	// current without triggering any broker logic.
	strats := newScoredStrategies(symbol)
	stochInd := stoch.New(14, 3)
	roundInd := round.New()
	for i := 0; i < warmupBars && i < n; i++ {
		for _, s := range strats {
			s.OnWarmUpCandle(ohlcSlice[i])
		}
		stochInd.Insert(ohlcSlice[i])
		roundInd.Insert(ohlcSlice[i])
	}

	var samples []ml.Sample
	for i := warmupBars; i < n-forwardDays; i++ {
		// Update all incremental state with bar i before reading any value.
		for _, s := range strats {
			s.OnWarmUpCandle(ohlcSlice[i])
		}
		stochInd.Insert(ohlcSlice[i])
		roundInd.Insert(ohlcSlice[i])

		c, cnext := closes[i], closes[i+forwardDays]
		if c <= 0 || cnext <= 0 {
			continue
		}

		// Build a candle window for strategies that inspect recent bars
		// directly (scalper needs 10, HeikinAshi needs 3, others ignore it).
		winStart := max(i-99, 0)
		window := ohlcSlice[winStart : i+1]

		var s ml.Sample
		s.Features[ml.FeatRSI] = rsiSeries[i] / 100.0
		s.Features[ml.FeatStochRSI] = stochK[i] / 100.0
		s.Features[ml.FeatRSIADX] = (rsiSeries[i] / 100.0) * (adxSeries[i] / 100.0)
		s.Features[ml.FeatSMA10] = relToSMA(c, sma10Series[i])
		s.Features[ml.FeatLowCandle] = hammer[i] / 100.0
		s.Features[ml.FeatEngulfing] = engulf[i] / 100.0
		s.Features[ml.FeatHarami] = harami[i] / 100.0
		s.Features[ml.FeatHA] = haSignals[i]
		s.Features[ml.FeatScalper] = bodyRatio(opens[i], highs[i], lows[i], c)
		s.Features[ml.FeatReturn1] = logRet(closes, i, 1)
		s.Features[ml.FeatReturn5] = logRet(closes, i, 5)
		s.Features[ml.FeatReturn20] = logRet(closes, i, 20)
		s.Features[ml.FeatZScore20] = returnZScore(closes, i, 20)
		// Strategy conviction scores (indices 13–22): each Score() reads the
		// indicator state already updated by OnWarmUpCandle above, so there
		// is no lookahead — all scores are based strictly on bars 0..i.
		s.Features[ml.FeatScoreDoji] = strats[0].Score(window)
		s.Features[ml.FeatScoreEngulf] = strats[1].Score(window)
		s.Features[ml.FeatScoreHarami] = strats[2].Score(window)
		s.Features[ml.FeatScoreHA] = strats[3].Score(window)
		s.Features[ml.FeatScoreLowCand] = strats[4].Score(window)
		s.Features[ml.FeatScoreRSI] = strats[5].Score(window)
		s.Features[ml.FeatScoreRSIADX] = strats[6].Score(window)
		s.Features[ml.FeatScoreScalper] = strats[7].Score(window)
		s.Features[ml.FeatScoreSMA10] = strats[8].Score(window)
		s.Features[ml.FeatScoreStochRSI] = strats[9].Score(window)
		// Fast Stochastic %K and %D from pkg/indicator/stoch (indices 23–24).
		if vals, err := stochInd.Value(); err == nil {
			s.Features[ml.FeatStochK] = vals[stoch.ValueK] / 100.0
			s.Features[ml.FeatStochD] = vals[stoch.ValueD] / 100.0
		}
		// Round-number proximity: position of close within the weak round band (index 25).
		if vals, err := roundInd.Value(); err == nil {
			lower := vals[round.LowerRoundNumberWeak]
			upper := vals[round.UpperRoundNumberWeak]
			if band := upper - lower; band > 0 {
				s.Features[ml.FeatRoundWeak] = (c - lower) / band
			}
		}
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
	for j := range window {
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
func bodyRatio(open, high, low, closePrice float64) float64 {
	rangeHL := high - low
	if rangeHL == 0 {
		return 0
	}
	return (closePrice - open) / rangeHL
}

// hammerSignals returns +1 when bar i is a hammer (small body at top, lower
// shadow > 2× body, upper shadow < body), -1 for an inverted hammer, 0 otherwise.
func hammerSignals(opens, highs, lows, closes []float64) []float64 {
	n := len(closes)
	out := make([]float64, n)
	for i := range n {
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
