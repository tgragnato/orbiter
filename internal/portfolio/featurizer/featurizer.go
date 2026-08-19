// Package featurizer converts raw EOD candle history for a portfolio into
// ml.Sample vectors ready for walk-forward training. It sits above both the
// features and ml packages to avoid import cycles.
package featurizer

import (
	"context"
	"fmt"
	"math"
	"time"

	talib "github.com/markcheno/go-talib"
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
	historyYears    = 8     // years of EOD history requested per symbol (~2016 bars)
	warmupBars      = 40    // leading bars consumed for indicator convergence and discarded
	forwardDays     = 5     // label horizon: 5-trading-day forward log-return (improves SNR over 1-day)
	eodHours        = 24    // hours in one EOD bar duration
	indicatorPeriod = 14    // shared RSI / ADX / StochRSI look-back period
	smaPeriod       = 10    // SMA look-back period
	stochSmooth     = 3     // StochRSI smoothing period
	scalerLookback  = 99    // maximum candle look-back window passed to strategy Score()
	indicatorScale  = 100.0 // indicator values are in 0-100; divide to normalise to 0-1
	return20Days    = 20    // 20-bar log-return and z-score look-back period
	haOHLCDivisor   = 4.0   // Heikin-Ashi close = (O+H+L+C) / 4
	haMidDivisor    = 2.0   // Heikin-Ashi open = (prevHaO + prevHaC) / 2
)

// watchlistLister is satisfied by any store that exposes watchlist symbol access.
// It is defined here (rather than importing portfolio) to avoid an import cycle.
type watchlistLister interface {
	ListWatchlistSymbols(ctx context.Context) ([]string, error)
}

// CurrentSamples returns the most recent feature vector for each TAA-eligible
// holding, including those with Quantity=0 (closed positions preserved in the
// portfolio). Zero-qty holdings are included so the TAA entry-signal path can
// compute conviction scores and suggest re-entry when the asset returns to trend.
// The Label field is populated but should be ignored — it is a current-bar
// feature vector, not a forward-return target.
//
// If the store also implements watchlistLister, watchlist symbols are included
// so the TAA engine can emit TypeBuy signals for assets not yet held.
//
func CurrentSamples(
	ctx context.Context,
	store portfolio.HoldingsStore,
	provider data.DataProvider,
) (map[string]ml.Sample, error) {
	holdings, err := store.ListHoldings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list holdings: %w", err)
	}

	now := time.Now().UTC()
	from := now.AddDate(-historyYears, 0, 0)

	result := make(map[string]ml.Sample)

	seen := make(map[string]bool)

	for _, holding := range holdings {
		if !holding.TAAEnabled || seen[holding.Symbol] {
			continue
		}

		seen[holding.Symbol] = true

		candles, fetchErr := provider.GetEOD(holding.Symbol, from, now)
		if fetchErr != nil || len(candles) < warmupBars+forwardDays+1 {
			continue
		}

		samples := samplesFromCandles(holding.Symbol, candles)
		if len(samples) == 0 {
			continue
		}

		result[holding.Symbol] = samples[len(samples)-1]
	}

	// Also score watchlist symbols so the TAA entry path can emit TypeBuy
	// signals for assets not yet in the portfolio.
	if lister, ok := store.(watchlistLister); ok {
		addCurrentWatchlistSamples(ctx, lister, provider, from, now, seen, result)
	}

	return result, nil
}

// addCurrentWatchlistSamples extends result with current-bar feature vectors
// for any watchlist symbol not already represented in seen.
func addCurrentWatchlistSamples(
	ctx context.Context,
	lister watchlistLister,
	provider data.DataProvider,
	from, now time.Time,
	seen map[string]bool,
	result map[string]ml.Sample,
) {
	syms, err := lister.ListWatchlistSymbols(ctx)
	if err != nil {
		return
	}

	for _, sym := range syms {
		if seen[sym] {
			continue
		}

		seen[sym] = true

		candles, fetchErr := provider.GetEOD(sym, from, now)
		if fetchErr != nil || len(candles) < warmupBars+forwardDays+1 {
			continue
		}

		samples := samplesFromCandles(sym, candles)
		if len(samples) == 0 {
			continue
		}

		result[sym] = samples[len(samples)-1]
	}
}

// ExtractMLSamples fetches historyYears of EOD candles for every active
// holding (Quantity > 0) and converts them into ml.Sample vectors ready for
// walk-forward training. Symbols whose fetch fails or whose history is too
// short are silently skipped so a partial portfolio still yields samples.
//
// If the store also implements watchlistLister, watchlist symbols are included
// in the training set so the model learns their patterns before entry.
//
func ExtractMLSamples(
	ctx context.Context,
	store portfolio.HoldingsStore,
	provider data.DataProvider,
) ([]ml.Sample, error) {
	holdings, err := store.ListHoldings(ctx)
	if err != nil {
		return nil, fmt.Errorf("list holdings: %w", err)
	}

	now := time.Now().UTC()
	from := now.AddDate(-historyYears, 0, 0)

	seen := make(map[string]bool)

	var all []ml.Sample

	for _, holding := range holdings {
		if holding.Quantity <= 0 || seen[holding.Symbol] {
			continue
		}

		seen[holding.Symbol] = true

		candles, fetchErr := provider.GetEOD(holding.Symbol, from, now)
		if fetchErr != nil || len(candles) < warmupBars+forwardDays+1 {
			continue
		}

		all = append(all, samplesFromCandles(holding.Symbol, candles)...)
	}

	// Include watchlist symbols in the training corpus so the model learns
	// their feature distributions before the user opens a position.
	if lister, ok := store.(watchlistLister); ok {
		all = appendWatchlistSamples(ctx, lister, provider, from, now, seen, all)
	}

	return all, nil
}

// appendWatchlistSamples extends all with training samples for any watchlist
// symbol not already represented in seen.
func appendWatchlistSamples(
	ctx context.Context,
	lister watchlistLister,
	provider data.DataProvider,
	from, now time.Time,
	seen map[string]bool,
	all []ml.Sample,
) []ml.Sample {
	syms, err := lister.ListWatchlistSymbols(ctx)
	if err != nil {
		return all
	}

	for _, sym := range syms {
		if seen[sym] {
			continue
		}

		seen[sym] = true

		candles, fetchErr := provider.GetEOD(sym, from, now)
		if fetchErr != nil || len(candles) < warmupBars+forwardDays+1 {
			continue
		}

		all = append(all, samplesFromCandles(sym, candles)...)
	}

	return all
}

// candleToOHLC converts a data.Candle (float64 fields) to *ohlc.OHLC
// (float64 fields) so strategy indicators can consume it.
// The resulting candle is treated as a closed EOD bar.
func candleToOHLC(candle data.Candle, symbol string) *ohlc.OHLC {
	price := candle.AdjustedClose
	if price <= 0 {
		price = candle.Close
	}

	ohlcBar := ohlc.New(symbol, candle.Time, eodHours*time.Hour, false)
	ohlcBar.Open = candle.Open
	ohlcBar.High = candle.High
	ohlcBar.Low = candle.Low
	ohlcBar.Close = price
	// EOD bars are always closed; ForceClose is required so pkg/indicator
	// implementations (rsi, adx, sma, stoch, stochrsi) don't silently drop
	// the bar when checking o.Closed().
	ohlcBar.ForceClose()

	return ohlcBar
}

// newScoredStrategies instantiates one of each ScoredStrategy implementation.
// All use 24 h as the candle duration (EOD data).
func newScoredStrategies(symbol string) []strategy.Strategy {
	dur := eodHours * time.Hour

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
//
//nolint:cyclop,funlen // inherent algorithmic complexity; extraction would obscure the feature-vector construction
func samplesFromCandles(symbol string, candles []data.Candle) []ml.Sample {
	numCandles := len(candles)

	opens := make([]float64, numCandles)
	highs := make([]float64, numCandles)
	lows := make([]float64, numCandles)
	closes := make([]float64, numCandles)

	for barIdx, candle := range candles {
		opens[barIdx] = candle.Open
		highs[barIdx] = candle.High
		lows[barIdx] = candle.Low
		closes[barIdx] = candle.AdjustedClose

		if closes[barIdx] <= 0 {
			closes[barIdx] = candle.Close
		}
	}

	rsiSeries := talib.Rsi(closes, indicatorPeriod)
	adxSeries := talib.Adx(highs, lows, closes, indicatorPeriod)
	sma10Series := talib.Sma(closes, smaPeriod)
	stochK, _ := talib.StochRsi(closes, indicatorPeriod, indicatorPeriod, stochSmooth, talib.SMA)
	engulf := engulfingSignals(opens, closes)
	harami := haramiSignals(opens, closes)
	hammer := hammerSignals(opens, highs, lows, closes)
	haSignals := heikinAshiSignals(opens, highs, lows, closes)

	// Convert all candles to *ohlc.OHLC once for strategy consumption.
	ohlcSlice := make([]*ohlc.OHLC, numCandles)
	for i, c := range candles {
		ohlcSlice[i] = candleToOHLC(c, symbol)
	}

	// Instantiate all ScoredStrategies and incremental indicators, then feed
	// the warm-up window. OnWarmUpCandle / Insert on every bar keeps state
	// current without triggering any broker logic.
	strats := newScoredStrategies(symbol)
	stochInd := stoch.New(indicatorPeriod, stochSmooth)
	roundInd := round.New()

	for warmIdx := 0; warmIdx < warmupBars && warmIdx < numCandles; warmIdx++ {
		for _, s := range strats {
			s.OnWarmUpCandle(ohlcSlice[warmIdx])
		}

		stochInd.Insert(ohlcSlice[warmIdx])
		roundInd.Insert(ohlcSlice[warmIdx])
	}

	var samples []ml.Sample

	for barIdx := warmupBars; barIdx < numCandles-forwardDays; barIdx++ {
		// Update all incremental state with bar barIdx before reading any value.
		for _, s := range strats {
			s.OnWarmUpCandle(ohlcSlice[barIdx])
		}

		stochInd.Insert(ohlcSlice[barIdx])
		roundInd.Insert(ohlcSlice[barIdx])

		closeVal, closeFwd := closes[barIdx], closes[barIdx+forwardDays]
		if closeVal <= 0 || closeFwd <= 0 {
			continue
		}

		// Build a candle window for strategies that inspect recent bars
		// directly (scalper needs 10, HeikinAshi needs 3, others ignore it).
		winStart := max(barIdx-scalerLookback, 0)
		window := ohlcSlice[winStart : barIdx+1]

		var sample ml.Sample

		sample.Features[ml.FeatRSI] = rsiSeries[barIdx] / indicatorScale
		sample.Features[ml.FeatStochRSI] = stochK[barIdx] / indicatorScale
		sample.Features[ml.FeatRSIADX] = (rsiSeries[barIdx] / indicatorScale) * (adxSeries[barIdx] / indicatorScale)
		sample.Features[ml.FeatSMA10] = relToSMA(closeVal, sma10Series[barIdx])
		sample.Features[ml.FeatLowCandle] = hammer[barIdx] / indicatorScale
		sample.Features[ml.FeatEngulfing] = engulf[barIdx] / indicatorScale
		sample.Features[ml.FeatHarami] = harami[barIdx] / indicatorScale
		sample.Features[ml.FeatHA] = haSignals[barIdx]
		sample.Features[ml.FeatScalper] = bodyRatio(opens[barIdx], highs[barIdx], lows[barIdx], closeVal)
		sample.Features[ml.FeatReturn1] = logRet(closes, barIdx, 1)
		sample.Features[ml.FeatReturn5] = logRet(closes, barIdx, forwardDays)
		sample.Features[ml.FeatReturn20] = logRet(closes, barIdx, return20Days)
		sample.Features[ml.FeatZScore20] = returnZScore(closes, barIdx, return20Days)
		// Strategy conviction scores (indices 13–22): each Score() reads the
		// indicator state already updated by OnWarmUpCandle above, so there
		// is no lookahead — all scores are based strictly on bars 0..barIdx.
		sample.Features[ml.FeatScoreDoji] = strats[0].Score(window)
		sample.Features[ml.FeatScoreEngulf] = strats[1].Score(window)
		sample.Features[ml.FeatScoreHarami] = strats[2].Score(window)
		sample.Features[ml.FeatScoreHA] = strats[3].Score(window)
		sample.Features[ml.FeatScoreLowCand] = strats[4].Score(window)
		sample.Features[ml.FeatScoreRSI] = strats[5].Score(window)
		sample.Features[ml.FeatScoreRSIADX] = strats[6].Score(window)
		sample.Features[ml.FeatScoreScalper] = strats[7].Score(window)
		sample.Features[ml.FeatScoreSMA10] = strats[8].Score(window)
		sample.Features[ml.FeatScoreStochRSI] = strats[9].Score(window)
		// Fast Stochastic %K and %D from pkg/indicator/stoch (indices 23–24).
		stochVals, stochErr := stochInd.Value()
		if stochErr == nil {
			sample.Features[ml.FeatStochK] = stochVals[stoch.ValueK] / indicatorScale
			sample.Features[ml.FeatStochD] = stochVals[stoch.ValueD] / indicatorScale
		}

		// Round-number proximity: position of close within the weak round band (index 25).
		roundVals, roundErr := roundInd.Value()
		if roundErr == nil {
			lower := roundVals[round.LowerRoundNumberWeak]
			upper := roundVals[round.UpperRoundNumberWeak]

			if band := upper - lower; band > 0 {
				sample.Features[ml.FeatRoundWeak] = (closeVal - lower) / band
			}
		}

		sample.Label = math.Log(closeFwd / closeVal)

		samples = append(samples, sample)
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

// logRet returns the log-return of closes[barIdx] relative to closes[barIdx-k].
func logRet(closes []float64, barIdx, k int) float64 {
	if barIdx < k || closes[barIdx-k] <= 0 {
		return 0
	}

	return math.Log(closes[barIdx] / closes[barIdx-k])
}

// returnZScore computes the z-score of the current 1-day log-return against
// the rolling window of the preceding `window` 1-day log-returns.
// Uses only past data so there is no lookahead bias.
func returnZScore(closes []float64, barIdx, window int) float64 {
	if barIdx < window+1 {
		return 0
	}

	rets := make([]float64, window)

	for retIdx := range window {
		idx := barIdx - window + retIdx
		if closes[idx] <= 0 || closes[idx+1] <= 0 {
			return 0
		}

		rets[retIdx] = math.Log(closes[idx+1] / closes[idx])
	}

	mean := features.Mean(rets)

	std := features.StdDev(rets, mean)
	if std == 0 {
		return 0
	}

	curr := logRet(closes, barIdx, 1)

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
// shadow > 2x body, upper shadow < body), -1 for an inverted hammer, 0 otherwise.
func hammerSignals(opens, highs, lows, closes []float64) []float64 {
	n := len(closes)

	out := make([]float64, n)

	for barIdx := range n {
		body := math.Abs(closes[barIdx] - opens[barIdx])
		lowerShadow := math.Min(opens[barIdx], closes[barIdx]) - lows[barIdx]
		upperShadow := highs[barIdx] - math.Max(opens[barIdx], closes[barIdx])

		if body == 0 {
			continue
		}

		if lowerShadow > 2*body && upperShadow < body {
			out[barIdx] = 1
		} else if upperShadow > 2*body && lowerShadow < body {
			out[barIdx] = -1
		}
	}

	return out
}

// engulfingSignals returns +1 for a bullish engulfing (bearish bar followed by
// a bullish bar whose body completely contains the prior body), -1 for bearish.
func engulfingSignals(opens, closes []float64) []float64 {
	n := len(closes)

	out := make([]float64, n)

	for barIdx := 1; barIdx < n; barIdx++ {
		prevBody := closes[barIdx-1] - opens[barIdx-1]
		currBody := closes[barIdx] - opens[barIdx]

		if prevBody < 0 && currBody > 0 &&
			closes[barIdx] >= opens[barIdx-1] && opens[barIdx] <= closes[barIdx-1] {
			out[barIdx] = 1
		} else if prevBody > 0 && currBody < 0 &&
			closes[barIdx] <= opens[barIdx-1] && opens[barIdx] >= closes[barIdx-1] {
			out[barIdx] = -1
		}
	}

	return out
}

// haramiSignals returns +1 for a bullish harami (bearish bar followed by a
// small bullish bar whose body fits inside the prior body), -1 for bearish.
func haramiSignals(opens, closes []float64) []float64 {
	n := len(closes)

	out := make([]float64, n)

	for barIdx := 1; barIdx < n; barIdx++ {
		prevLo := math.Min(opens[barIdx-1], closes[barIdx-1])
		prevHi := math.Max(opens[barIdx-1], closes[barIdx-1])
		currLo := math.Min(opens[barIdx], closes[barIdx])
		currHi := math.Max(opens[barIdx], closes[barIdx])
		prevBearish := closes[barIdx-1] < opens[barIdx-1]
		currBullish := closes[barIdx] > opens[barIdx]

		if prevBearish && currBullish && currLo >= prevLo && currHi <= prevHi {
			out[barIdx] = 1
		} else if !prevBearish && !currBullish && currLo >= prevLo && currHi <= prevHi {
			out[barIdx] = -1
		}
	}

	return out
}

// heikinAshiSignals computes (haClose - haOpen) / haOpen for each bar.
// The result is 0 for the first bar (no prior HA candle available).
func heikinAshiSignals(opens, highs, lows, closes []float64) []float64 {
	numCandles := len(closes)

	signals := make([]float64, numCandles)

	if numCandles == 0 {
		return signals
	}

	haO := opens[0]
	haC := (opens[0] + highs[0] + lows[0] + closes[0]) / haOHLCDivisor

	for barIdx := 1; barIdx < numCandles; barIdx++ {
		prevHaO, prevHaC := haO, haC
		haC = (opens[barIdx] + highs[barIdx] + lows[barIdx] + closes[barIdx]) / haOHLCDivisor
		haO = (prevHaO + prevHaC) / haMidDivisor

		if haO > 0 {
			signals[barIdx] = (haC - haO) / haO
		}
	}

	return signals
}
