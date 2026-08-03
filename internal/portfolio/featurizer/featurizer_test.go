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

// --- fakes ---

type fakeStore struct {
	holdings []portfolio.Holding
	err      error
}

func (f *fakeStore) ListHoldings(_ context.Context) ([]portfolio.Holding, error) {
	return f.holdings, f.err
}
func (f *fakeStore) ToggleAllocation(_ context.Context, _ int64) error      { return nil }
func (f *fakeStore) ToggleTAAEnabled(_ context.Context, _ string) error     { return nil }
func (f *fakeStore) TotalRealizedPnL(_ context.Context) (float64, error)    { return 0, nil }

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

// syntheticCandles builds n daily candles with a simple linear price trend
// starting at basePrice, so all indicators have valid, non-trivial input.
func syntheticCandles(n int, basePrice float64) []data.Candle {
	candles := make([]data.Candle, n)
	price := basePrice
	for i := range candles {
		open := price
		close := price * (1 + 0.002*float64(i%5-2)) // small oscillation
		high := math.Max(open, close) * 1.005
		low := math.Min(open, close) * 0.995
		candles[i] = data.Candle{
			Ticker:        "TEST",
			Time:          time.Now().AddDate(0, 0, -n+i),
			Open:          open,
			High:          high,
			Low:           low,
			Close:         close,
			AdjustedClose: close,
		}
		price = close
	}
	return candles
}

// --- tests ---

func TestExtractMLSamplesEmptyHoldings(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	provider := &fakeProvider{candles: map[string][]data.Candle{}}

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
	store := &fakeStore{holdings: []portfolio.Holding{
		{Symbol: "VWCE.MI", Quantity: 0},
	}}
	provider := &fakeProvider{candles: map[string][]data.Candle{
		"VWCE.MI": syntheticCandles(200, 100),
	}}

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
	store := &fakeStore{holdings: []portfolio.Holding{
		{Symbol: "VWCE.MI", Quantity: 10},
		{Symbol: "VWCE.MI", Quantity: 5}, // duplicate
	}}
	candles := syntheticCandles(200, 100)
	provider := &fakeProvider{candles: map[string][]data.Candle{"VWCE.MI": candles}}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Single fetch, not doubled
	single, _ := ExtractMLSamples(context.Background(), &fakeStore{holdings: []portfolio.Holding{
		{Symbol: "VWCE.MI", Quantity: 10},
	}}, provider)
	if len(samples) != len(single) {
		t.Fatalf("dedup failed: got %d samples for duplicate symbol, want %d", len(samples), len(single))
	}
}

func TestExtractMLSamplesSkipsShortHistory(t *testing.T) {
	t.Parallel()
	store := &fakeStore{holdings: []portfolio.Holding{
		{Symbol: "SHORT", Quantity: 1},
	}}
	provider := &fakeProvider{candles: map[string][]data.Candle{
		"SHORT": syntheticCandles(warmupBars, 100), // exactly warmupBars — too short
	}}

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
	store := &fakeStore{holdings: []portfolio.Holding{
		{Symbol: "ETF1", Quantity: 10},
	}}
	provider := &fakeProvider{candles: map[string][]data.Candle{
		"ETF1": syntheticCandles(nCandles, 100),
	}}

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
	store := &fakeStore{holdings: []portfolio.Holding{
		{Symbol: "ETF1", Quantity: 1},
	}}
	provider := &fakeProvider{candles: map[string][]data.Candle{
		"ETF1": syntheticCandles(200, 50),
	}}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil || len(samples) == 0 {
		t.Fatalf("expected samples, got err=%v len=%d", err, len(samples))
	}
	if len(samples[0].Features) != ml.FeatureCount() {
		t.Fatalf("feature count = %d, want %d", len(samples[0].Features), ml.FeatureCount())
	}
}

func TestExtractMLSamplesLabelIsForwardReturn(t *testing.T) {
	t.Parallel()
	// Build candles with known prices so we can verify the label.
	n := warmupBars + 5
	candles := make([]data.Candle, n)
	for i := range candles {
		p := 100.0 + float64(i)
		candles[i] = data.Candle{
			Open: p, High: p * 1.01, Low: p * 0.99,
			Close: p, AdjustedClose: p,
			Time: time.Now().AddDate(0, 0, -n+i),
		}
	}

	store := &fakeStore{holdings: []portfolio.Holding{{Symbol: "X", Quantity: 1}}}
	provider := &fakeProvider{candles: map[string][]data.Candle{"X": candles}}

	samples, err := ExtractMLSamples(context.Background(), store, provider)
	if err != nil || len(samples) == 0 {
		t.Fatalf("expected samples, got err=%v len=%d", err, len(samples))
	}

	// First sample covers candles[warmupBars]; label = log(close[warmupBars+1] / close[warmupBars])
	wantLabel := math.Log(candles[warmupBars+forwardDays].AdjustedClose / candles[warmupBars].AdjustedClose)
	if math.Abs(samples[0].Label-wantLabel) > 1e-9 {
		t.Fatalf("label = %f, want %f", samples[0].Label, wantLabel)
	}
}

func TestExtractMLSamplesMultipleSymbols(t *testing.T) {
	t.Parallel()
	store := &fakeStore{holdings: []portfolio.Holding{
		{Symbol: "A", Quantity: 1},
		{Symbol: "B", Quantity: 1},
	}}
	provider := &fakeProvider{candles: map[string][]data.Candle{
		"A": syntheticCandles(150, 100),
		"B": syntheticCandles(150, 200),
	}}

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
	s1 := samplesFromCandles("TEST", candles)

	// Mutating a future candle must not change earlier samples.
	candles[len(candles)-1].Close *= 100
	s2 := samplesFromCandles("TEST", candles)
	if len(s1) == 0 || len(s2) == 0 {
		t.Skip("no samples produced")
	}
	if s1[0].Features[ml.FeatHA] != s2[0].Features[ml.FeatHA] {
		t.Errorf("HA signal changed after mutating a future candle: before=%f after=%f",
			s1[0].Features[ml.FeatHA], s2[0].Features[ml.FeatHA])
	}
}

func TestBodyRatio(t *testing.T) {
	t.Parallel()
	tests := []struct {
		open, high, low, close float64
		want                   float64
	}{
		{100, 110, 90, 108, 0.4},   // body=(8), range=(20) → 8/20=0.4
		{100, 110, 90, 92, -0.4},  // body=(-8), range=(20) → -8/20=-0.4
		{100, 100, 100, 100, 0},   // doji: range=0
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

func TestEngulfingSignals(t *testing.T) {
	t.Parallel()
	// Bullish engulfing: bearish bar [110→100] then bullish bar [99→112]
	opens  := []float64{110, 99}
	closes := []float64{100, 112}
	sig := engulfingSignals(opens, closes)
	if sig[1] != 1 {
		t.Errorf("bullish engulfing signal = %f, want 1", sig[1])
	}

	// Bearish engulfing: bullish [90→100] then bearish [101→89]
	opens2  := []float64{90, 101}
	closes2 := []float64{100, 89}
	sig2 := engulfingSignals(opens2, closes2)
	if sig2[1] != -1 {
		t.Errorf("bearish engulfing signal = %f, want -1", sig2[1])
	}
}

func TestHammerSignals(t *testing.T) {
	t.Parallel()
	// Hammer: small body at top (open≈close), long lower shadow
	opens  := []float64{105}
	closes := []float64{106}
	highs  := []float64{106.5}
	lows   := []float64{100} // lower shadow = 5, body = 1 → hammer
	sig := hammerSignals(opens, highs, lows, closes)
	if sig[0] != 1 {
		t.Errorf("hammer signal = %f, want 1", sig[0])
	}
}
