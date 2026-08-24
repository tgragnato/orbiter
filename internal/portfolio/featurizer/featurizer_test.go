//nolint:testpackage // accesses unexported symbols: samplesFromCandles, warmupBars, forwardDays
package featurizer

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/internal/ml"
	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/portfolio/data"
)

const (
	symbolETF1   = "ETF1"
	symbolVWCEMI = "VWCE.MI"
)

// --- fakes ---

type fakeStore struct {
	holdings []portfolio.Holding
	err      error
}

func (f *fakeStore) ListHoldings(_ context.Context) ([]portfolio.Holding, error) {
	return f.holdings, f.err
}

func (f *fakeStore) ToggleAllocation(_ context.Context, _ int64) error   { return nil }
func (f *fakeStore) ToggleTAAEnabled(_ context.Context, _ string) error  { return nil }
func (f *fakeStore) TotalRealizedPnL(_ context.Context) (float64, error) { return 0, nil }

type fakeProvider struct {
	candles map[string][]data.Candle
	err     error
}

func (f *fakeProvider) GetEOD(ticker string, _, _ time.Time) ([]data.Candle, error) {
	if f.err != nil {
		return nil, f.err
	}

	return f.candles[ticker], nil
}

// syntheticCandles builds count daily candles with a simple linear price trend
// starting at basePrice, so all indicators have valid, non-trivial input.
func syntheticCandles(count int, basePrice float64) []data.Candle {
	candles := make([]data.Candle, count)
	startPrice := basePrice

	for idx := range candles {
		open := startPrice
		closePrice := startPrice * (1 + 0.002*float64(idx%5-2)) // small oscillation
		high := math.Max(open, closePrice) * 1.005
		low := math.Min(open, closePrice) * 0.995
		candles[idx] = data.Candle{
			Ticker:        "TEST",
			Time:          time.Now().AddDate(0, 0, -count+idx),
			Open:          open,
			High:          high,
			Low:           low,
			Close:         closePrice,
			AdjustedClose: closePrice,
			Volume:        0,
			SplitFactor:   0,
			CashDividend:  0,
			Currency:      "",
		}
		startPrice = closePrice
	}

	return candles
}

// --- tests ---

func TestCurrentSamplesIncludesZeroQtyTAAEnabled(t *testing.T) {
	t.Parallel()

	// Closed position (Quantity=0) with TAAEnabled=true must be included so
	// the TAA entry-signal path can compute conviction for re-entry.
	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID:             0,
				Symbol:         symbolETF1,
				Quantity:       0,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     true,
				Currency:       "",
			},
		},
		err: nil,
	}
	provider := &fakeProvider{
		candles: map[string][]data.Candle{
			symbolETF1: syntheticCandles(200, 100),
		},
		err: nil,
	}

	samples, err := CurrentSamples(context.Background(), store, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := samples[symbolETF1]; !ok {
		t.Error("expected CurrentSamples to include zero-qty TAAEnabled holding for re-entry conviction")
	}
}

func TestCurrentSamplesExcludesTAADisabled(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID:             0,
				Symbol:         symbolETF1,
				Quantity:       10,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
		},
		err: nil,
	}
	provider := &fakeProvider{
		candles: map[string][]data.Candle{
			symbolETF1: syntheticCandles(200, 100),
		},
		err: nil,
	}

	samples, err := CurrentSamples(context.Background(), store, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := samples[symbolETF1]; ok {
		t.Error("expected CurrentSamples to exclude TAAEnabled=false holding")
	}
}

func TestExtractMLSamplesEmptyHoldings(t *testing.T) {
	t.Parallel()

	store := &fakeStore{holdings: nil, err: nil}
	provider := &fakeProvider{candles: map[string][]data.Candle{}, err: nil}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(samples) != 0 {
		t.Fatalf("expected 0 samples, got %d", len(samples))
	}
}

func TestExtractMLSamplesSkipsZeroQty(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID:             0,
				Symbol:         symbolVWCEMI,
				Quantity:       0,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
		},
		err: nil,
	}
	provider := &fakeProvider{
		candles: map[string][]data.Candle{
			symbolVWCEMI: syntheticCandles(200, 100),
		},
		err: nil,
	}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(samples) != 0 {
		t.Fatalf("expected 0 samples for zero-qty holding, got %d", len(samples))
	}
}

func TestExtractMLSamplesDeduplicatesSymbols(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID:             0,
				Symbol:         symbolVWCEMI,
				Quantity:       10,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
			{
				ID:             0,
				Symbol:         symbolVWCEMI,
				Quantity:       5, // duplicate
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
		},
		err: nil,
	}
	candles := syntheticCandles(200, 100)
	provider := &fakeProvider{candles: map[string][]data.Candle{symbolVWCEMI: candles}, err: nil}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Single fetch, not doubled.
	single, _ := ExtractMLSamples(context.Background(), &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID:             0,
				Symbol:         symbolVWCEMI,
				Quantity:       10,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
		},
		err: nil,
	}, provider)

	if len(samples) != len(single) {
		t.Fatalf("dedup failed: got %d samples for duplicate symbol, want %d", len(samples), len(single))
	}
}

func TestExtractMLSamplesSkipsShortHistory(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID:             0,
				Symbol:         "SHORT",
				Quantity:       1,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
		},
		err: nil,
	}
	provider := &fakeProvider{
		candles: map[string][]data.Candle{
			"SHORT": syntheticCandles(warmupBars, 100), // exactly warmupBars — too short
		},
		err: nil,
	}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(samples) != 0 {
		t.Fatalf("expected 0 samples for short history, got %d", len(samples))
	}
}

func TestExtractMLSamplesProducesSamples(t *testing.T) {
	t.Parallel()

	const nCandles = 200

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID:             0,
				Symbol:         symbolETF1,
				Quantity:       10,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
		},
		err: nil,
	}
	provider := &fakeProvider{
		candles: map[string][]data.Candle{
			symbolETF1: syntheticCandles(nCandles, 100),
		},
		err: nil,
	}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := nCandles - warmupBars - forwardDays
	if len(samples) != want {
		t.Fatalf("got %d samples, want %d", len(samples), want)
	}
}

func TestExtractMLSamplesFeatureCount(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID:             0,
				Symbol:         symbolETF1,
				Quantity:       1,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
		},
		err: nil,
	}
	provider := &fakeProvider{
		candles: map[string][]data.Candle{
			symbolETF1: syntheticCandles(200, 50),
		},
		err: nil,
	}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil || len(samples) == 0 {
		t.Fatalf("expected samples, got err=%v len=%d", err, len(samples))
	}

	const wantFeatures = 30 // featureCount in internal/ml/sample.go

	if len(samples[0].Features) != wantFeatures {
		t.Fatalf("feature count = %d, want %d", len(samples[0].Features), wantFeatures)
	}
}

func TestExtractMLSamplesLabelIsForwardReturn(t *testing.T) {
	t.Parallel()

	// Build candles with known prices so we can verify the label.
	// Need warmupBars + forwardDays + 1 to produce exactly one sample.
	numBars := warmupBars + forwardDays + 1
	candles := make([]data.Candle, numBars)

	for barIdx := range candles {
		price := 100.0 + float64(barIdx)
		candles[barIdx] = data.Candle{
			Ticker:        "",
			Open:          price,
			High:          price * 1.01,
			Low:           price * 0.99,
			Close:         price,
			AdjustedClose: price,
			Time:          time.Now().AddDate(0, 0, -numBars+barIdx),
			Volume:        0,
			SplitFactor:   0,
			CashDividend:  0,
			Currency:      "",
		}
	}

	store := &fakeStore{
		holdings: []portfolio.Holding{{
			ID:             0,
			Symbol:         "X",
			Quantity:       1,
			MarketPrice:    0,
			PMC:            0,
			AllocationType: "",
			TAAEnabled:     false,
			Currency:       "",
		}},
		err: nil,
	}
	provider := &fakeProvider{candles: map[string][]data.Candle{"X": candles}, err: nil}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil || len(samples) == 0 {
		t.Fatalf("expected samples, got err=%v len=%d", err, len(samples))
	}

	// First sample covers candles[warmupBars]; label = log(close[warmupBars+forwardDays] / close[warmupBars]).
	wantLabel := math.Log(candles[warmupBars+forwardDays].AdjustedClose / candles[warmupBars].AdjustedClose)
	if math.Abs(samples[0].Label-wantLabel) > 1e-9 {
		t.Fatalf("label = %f, want %f", samples[0].Label, wantLabel)
	}
}

func TestExtractMLSamplesMultipleSymbols(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID:             0,
				Symbol:         "A",
				Quantity:       1,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
			{
				ID:             0,
				Symbol:         "B",
				Quantity:       1,
				MarketPrice:    0,
				PMC:            0,
				AllocationType: "",
				TAAEnabled:     false,
				Currency:       "",
			},
		},
		err: nil,
	}
	provider := &fakeProvider{
		candles: map[string][]data.Candle{
			"A": syntheticCandles(150, 100),
			"B": syntheticCandles(150, 200),
		},
		err: nil,
	}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := 2 * (150 - warmupBars - forwardDays)
	if len(samples) != want {
		t.Fatalf("got %d samples, want %d", len(samples), want)
	}
}

func TestSamplesFromCandlesHeikinAshiNoLookahead(t *testing.T) {
	t.Parallel()

	candles := syntheticCandles(warmupBars+10, 100)

	samplesOrig := samplesFromCandles("TEST", candles, nil)

	// Mutating a future candle must not change earlier samples.
	candles[len(candles)-1].Close *= 100

	samplesMutated := samplesFromCandles("TEST", candles, nil)
	if len(samplesOrig) == 0 || len(samplesMutated) == 0 {
		t.Skip("no samples produced")
	}

	if samplesOrig[0].Features[ml.FeatHA] != samplesMutated[0].Features[ml.FeatHA] {
		t.Errorf("HA signal changed after mutating a future candle: before=%f after=%f",
			samplesOrig[0].Features[ml.FeatHA], samplesMutated[0].Features[ml.FeatHA])
	}
}

func TestBodyRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		open, high, low, close float64
		want                   float64
	}{
		{100, 110, 90, 108, 0.4}, // body=(8), range=(20) -> 8/20=0.4
		{100, 110, 90, 92, -0.4}, // body=(-8), range=(20) -> -8/20=-0.4
		{100, 100, 100, 100, 0},  // doji: range=0
	}

	for _, tc := range tests {
		got := bodyRatio(tc.open, tc.high, tc.low, tc.close)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("bodyRatio(%v,%v,%v,%v) = %f, want %f",
				tc.open, tc.high, tc.low, tc.close, got, tc.want)
		}
	}
}

func TestLogRet(t *testing.T) {
	t.Parallel()

	prices := []float64{100, 110, 121}

	got := logRet(prices, 2, 2)

	want := math.Log(121.0 / 100.0)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("logRet = %f, want %f", got, want)
	}

	if logRet(prices, 1, 5) != 0 {
		t.Error("out-of-bounds k should return 0")
	}
}

// growthCandles builds count daily candles whose adjusted close grows by a
// constant daily log-return, so multi-horizon momentum has a closed form.
func growthCandles(count int, basePrice, dailyLogRet float64) []data.Candle {
	candles := make([]data.Candle, count)

	for idx := range candles {
		price := basePrice * math.Exp(dailyLogRet*float64(idx))
		candles[idx] = data.Candle{
			Ticker:        "TEST",
			Time:          time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -count+idx),
			Open:          price,
			High:          price * 1.001,
			Low:           price * 0.999,
			Close:         price,
			AdjustedClose: price,
			Volume:        0,
			SplitFactor:   0,
			CashDividend:  0,
			Currency:      "",
		}
	}

	return candles
}

func TestSamplesFromCandlesMediumTermMomentum(t *testing.T) {
	t.Parallel()

	const dailyLogRet = 0.001

	candles := growthCandles(warmupBars+forwardDays+1, 100, dailyLogRet)

	samples := samplesFromCandles("TEST", candles, nil)
	if len(samples) == 0 {
		t.Fatal("expected samples")
	}

	first := samples[0]

	want60 := dailyLogRet * 60
	if math.Abs(first.Features[ml.FeatReturn60]-want60) > 1e-9 {
		t.Errorf("FeatReturn60 = %f, want %f", first.Features[ml.FeatReturn60], want60)
	}

	want120 := dailyLogRet * 120
	if math.Abs(first.Features[ml.FeatReturn120]-want120) > 1e-9 {
		t.Errorf("FeatReturn120 = %f, want %f", first.Features[ml.FeatReturn120], want120)
	}
}

func TestSamplesFromCandlesNilBenchmarkZeroRelReturns(t *testing.T) {
	t.Parallel()

	candles := growthCandles(warmupBars+forwardDays+1, 100, 0.001)

	samples := samplesFromCandles("TEST", candles, nil)
	if len(samples) == 0 {
		t.Fatal("expected samples")
	}

	if samples[0].Features[ml.FeatRelReturn20] != 0 || samples[0].Features[ml.FeatRelReturn60] != 0 {
		t.Errorf("nil benchmark must yield zero relative returns, got %f and %f",
			samples[0].Features[ml.FeatRelReturn20], samples[0].Features[ml.FeatRelReturn60])
	}
}

func TestSamplesFromCandlesRelativeReturnVsBenchmark(t *testing.T) {
	t.Parallel()

	const (
		assetDaily = 0.002
		benchDaily = 0.001
	)

	numBars := warmupBars + forwardDays + 1
	assetCandles := growthCandles(numBars, 100, assetDaily)
	benchCandles := growthCandles(numBars, 50, benchDaily)

	holdings := []portfolio.Holding{{
		ID:             0,
		Symbol:         "BENCH",
		Quantity:       1,
		MarketPrice:    0,
		PMC:            0,
		AllocationType: "",
		TAAEnabled:     false,
		Currency:       "",
	}}
	provider := &fakeProvider{
		candles: map[string][]data.Candle{"BENCH": benchCandles},
		err:     nil,
	}

	bench := buildBenchmark(holdings, provider, time.Time{}, time.Now())
	if bench == nil {
		t.Fatal("expected non-nil benchmark")
	}

	samples := samplesFromCandles("TEST", assetCandles, bench)
	if len(samples) == 0 {
		t.Fatal("expected samples")
	}

	first := samples[0]

	want20 := (assetDaily - benchDaily) * 20
	if math.Abs(first.Features[ml.FeatRelReturn20]-want20) > 1e-6 {
		t.Errorf("FeatRelReturn20 = %f, want %f", first.Features[ml.FeatRelReturn20], want20)
	}

	want60 := (assetDaily - benchDaily) * 60
	if math.Abs(first.Features[ml.FeatRelReturn60]-want60) > 1e-6 {
		t.Errorf("FeatRelReturn60 = %f, want %f", first.Features[ml.FeatRelReturn60], want60)
	}
}

func TestBuildBenchmarkFallsBackWhenPortfolioEmpty(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{
		candles: map[string][]data.Candle{
			"EXUS.MI": growthCandles(200, 100, 0.001),
		},
		err: nil,
	}

	bench := buildBenchmark(nil, provider, time.Time{}, time.Now())
	if bench == nil {
		t.Fatal("expected fallback benchmark from EXUS.MI, got nil")
	}
}

func TestBuildBenchmarkNilWhenNoDataAvailable(t *testing.T) {
	t.Parallel()

	provider := &fakeProvider{candles: map[string][]data.Candle{}, err: nil}

	bench := buildBenchmark(nil, provider, time.Time{}, time.Now())
	if bench != nil {
		t.Fatal("expected nil benchmark when no series is available")
	}
}

func TestBuildBenchmarkEqualWeightsTwoHoldings(t *testing.T) {
	t.Parallel()

	const numBars = 100

	holdings := []portfolio.Holding{
		{
			ID:             0,
			Symbol:         "A",
			Quantity:       1,
			MarketPrice:    0,
			PMC:            0,
			AllocationType: "",
			TAAEnabled:     false,
			Currency:       "",
		},
		{
			ID:             0,
			Symbol:         "B",
			Quantity:       1,
			MarketPrice:    0,
			PMC:            0,
			AllocationType: "",
			TAAEnabled:     false,
			Currency:       "",
		},
	}
	provider := &fakeProvider{
		candles: map[string][]data.Candle{
			"A": growthCandles(numBars, 100, 0.002),
			"B": growthCandles(numBars, 100, 0.000),
		},
		err: nil,
	}

	bench := buildBenchmark(holdings, provider, time.Time{}, time.Now())
	if bench == nil {
		t.Fatal("expected non-nil benchmark")
	}

	// Equal-weight mean of daily log-returns: (0.002 + 0.000) / 2 per day over 20 days.
	from := bench.dates[len(bench.dates)-21]
	to := bench.dates[len(bench.dates)-1]

	got, ok := bench.logRetBetween(from, to)
	if !ok {
		t.Fatal("expected benchmark to cover the window")
	}

	want := 0.001 * 20
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("benchmark 20-day log-return = %f, want %f", got, want)
	}
}

func TestEngulfingSignals(t *testing.T) {
	t.Parallel()

	// Bullish engulfing: bearish bar [110->100] then bullish bar [99->112]
	opens := []float64{110, 99}
	closes := []float64{100, 112}

	sig := engulfingSignals(opens, closes)
	if sig[1] != 1 {
		t.Errorf("bullish engulfing signal = %f, want 1", sig[1])
	}

	// Bearish engulfing: bullish [90->100] then bearish [101->89]
	opens2 := []float64{90, 101}
	closes2 := []float64{100, 89}

	sig2 := engulfingSignals(opens2, closes2)
	if sig2[1] != -1 {
		t.Errorf("bearish engulfing signal = %f, want -1", sig2[1])
	}
}

func TestHammerSignals(t *testing.T) {
	t.Parallel()

	// Hammer: small body at top (open~close), long lower shadow.
	opens := []float64{105}
	closes := []float64{106}
	highs := []float64{106.5}
	lows := []float64{100} // lower shadow = 5, body = 1 -> hammer

	sig := hammerSignals(opens, highs, lows, closes)
	if sig[0] != 1 {
		t.Errorf("hammer signal = %f, want 1", sig[0])
	}
}
