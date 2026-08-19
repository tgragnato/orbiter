package lowcandle_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/strategy/lowcandle"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// generateTestCandles creates valid OHLC candles with ForceClose() applied.
//
//nolint:unparam // basePrice is always 100.0 in tests; kept parametric for readability
func generateTestCandles(count int, basePrice float64) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	for idx := range count {
		// Alternate prices slightly to create a high/low range
		offset := float64(idx % 3)
		openPrice := basePrice + offset
		closePrice := openPrice + 0.5
		high := openPrice + 2.0
		low := openPrice - 2.0

		candle := &ohlc.OHLC{
			Instrument: "",
			Open:       openPrice,
			High:       high,
			HighTime:   time.Time{},
			Low:        low,
			LowTime:    time.Time{},
			Close:      closePrice,
			Start:      start.Add(time.Duration(idx) * time.Hour),
			End:        start.Add(time.Duration(idx+1) * time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		candle.ForceClose()
		candles[idx] = candle
	}

	return candles
}

func TestLowCandle_Name(t *testing.T) {
	t.Parallel()

	lc := lowcandle.New("test", time.Minute*60)
	if strategy.NameLowCandle != lc.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameLowCandle, lc.Name())
	}
}

func TestLowCandle_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	lc := lowcandle.New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := lc.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

//nolint:funlen // test function covers multiple subtests; splitting would reduce readability
func TestLowCandle_Score_And_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	t.Run("No WarmUp (Empty Buffers)", func(t *testing.T) {
		t.Parallel()

		lc := lowcandle.New("test", time.Minute*60)
		candles := generateTestCandles(1, 100.0)

		got := lc.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when circular buffers are uninitialized", got)
		}
	})

	t.Run("Inside Historical Range (Neutral Score)", func(t *testing.T) {
		t.Parallel()

		lowCandleInst := lowcandle.New("test", time.Minute*60)
		warmUp := generateTestCandles(7, 100.0)

		for _, c := range warmUp {
			lowCandleInst.OnWarmUpCandle(c)
		}

		// Close price inside range
		testCandles := []*ohlc.OHLC{warmUp[0]}

		got := lowCandleInst.Score(testCandles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 for close price inside historical range", got)
		}
	})

	t.Run("Breakout Below Low (Positive Buy Score)", func(t *testing.T) {
		t.Parallel()

		lowCandleInst := lowcandle.New("test", time.Minute*60)
		warmUp := generateTestCandles(7, 100.0)

		for _, c := range warmUp {
			lowCandleInst.OnWarmUpCandle(c)
		}

		// Historical Low is 98.0, High is 104.5 -> totalRange = 6.5
		// Create a candle with Close = 91.5 (below min low 98.0)
		breakoutCandle := &ohlc.OHLC{
			Instrument: "",
			Open:       93.0,
			High:       94.0,
			HighTime:   time.Time{},
			Low:        90.0,
			LowTime:    time.Time{},
			Close:      91.5,
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		breakoutCandle.ForceClose()

		got := lowCandleInst.Score([]*ohlc.OHLC{breakoutCandle})
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 for breakout below historical low", got)
		}
	})

	t.Run("Breakout Above High (Negative Sell Score)", func(t *testing.T) {
		t.Parallel()

		lowCandleInst := lowcandle.New("test", time.Minute*60)
		warmUp := generateTestCandles(7, 100.0)

		for _, c := range warmUp {
			lowCandleInst.OnWarmUpCandle(c)
		}

		// Create a candle with Close = 110.0 (above max high 104.5)
		breakoutCandle := &ohlc.OHLC{
			Instrument: "",
			Open:       108.0,
			High:       111.0,
			HighTime:   time.Time{},
			Low:        107.0,
			LowTime:    time.Time{},
			Close:      110.0,
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		breakoutCandle.ForceClose()

		got := lowCandleInst.Score([]*ohlc.OHLC{breakoutCandle})
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 for breakout above historical high", got)
		}
	})
}
