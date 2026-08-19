//nolint:testpackage // uses unexported helpers generateTestCandles, generateSidewaysCandles
package rsiadx

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// generateTestCandles creates valid OHLC candles with correct High/Low boundaries and calls ForceClose().
func generateTestCandles(count int, startPrice, step float64) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	for idx := range count {
		open := startPrice + float64(idx)*step
		closePrice := open + step
		high := open + 1.0
		low := open - 1.0

		if step > 0 {
			high = closePrice + 1.0
			low = open - 1.0
		} else if step < 0 {
			high = open + 1.0
			low = closePrice - 1.0
		}

		candle := &ohlc.OHLC{
			Instrument: "",
			Open:       open,
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

// generateSidewaysCandles creates oscillating OHLC candles to simulate a range-bound market (low ADX).
func generateSidewaysCandles(count int, basePrice float64) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	for idx := range count {
		open := basePrice

		closePrice := basePrice
		if idx%2 == 0 {
			closePrice += 1.0
		} else {
			closePrice -= 1.0
		}

		candle := &ohlc.OHLC{
			Instrument: "",
			Open:       open,
			High:       basePrice + 2.0,
			HighTime:   time.Time{},
			Low:        basePrice - 2.0,
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

func TestRSIADX_Name(t *testing.T) {
	t.Parallel()

	rsiAdx := New("test", time.Minute*60)
	if strategy.NameRSIADX != rsiAdx.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameRSIADX, rsiAdx.Name())
	}
}

func TestRSIADX_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	rsiAdx := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := rsiAdx.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

//nolint:funlen // test function covers multiple subtests
func TestRSIADX_Score(t *testing.T) {
	t.Parallel()

	t.Run("No WarmUp (Uninitialized Indicators)", func(t *testing.T) {
		t.Parallel()

		rsiAdx := New("test", time.Minute*60)
		candles := generateTestCandles(5, 100.0, 1.0)

		got := rsiAdx.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when indicators are not ready", got)
		}
	})

	t.Run("Sideways Market (Weak ADX)", func(t *testing.T) {
		t.Parallel()

		rsiAdx := New("test", time.Minute*60)
		// Oscillating candles keep ADX below threshold (< 35)
		candles := generateSidewaysCandles(100, 100.0)

		for _, candle := range candles {
			rsiAdx.OnWarmUpCandle(candle)
		}

		got := rsiAdx.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when ADX is below threshold", got)
		}
	})

	t.Run("Strong Bullish Trend (Overbought / Negative Conviction)", func(t *testing.T) {
		t.Parallel()

		rsiAdx := New("test", time.Minute*60)
		// 100 candles rising steadily -> high ADX and overbought RSI
		candles := generateTestCandles(100, 100.0, 1.0)

		for _, candle := range candles {
			rsiAdx.OnWarmUpCandle(candle)
		}

		got := rsiAdx.Score(candles)
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 (SHORT signal / sell conviction)", got)
		}
	})

	t.Run("Strong Bearish Trend (Oversold / Positive Conviction)", func(t *testing.T) {
		t.Parallel()

		rsiAdx := New("test", time.Minute*60)
		// 100 candles falling steadily -> high ADX and oversold RSI
		candles := generateTestCandles(100, 500.0, -1.0)

		for _, candle := range candles {
			rsiAdx.OnWarmUpCandle(candle)
		}

		got := rsiAdx.Score(candles)
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 (LONG signal / buy conviction)", got)
		}
	})
}
