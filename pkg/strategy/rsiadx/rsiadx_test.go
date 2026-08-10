package rsiadx

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// generateTestCandles creates valid OHLC candles with correct High/Low boundaries and calls ForceClose().
func generateTestCandles(count int, startPrice float64, step float64) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < count; i++ {
		open := startPrice + float64(i)*step
		close := open + step
		high := open + 1.0
		low := open - 1.0

		if step > 0 {
			high = close + 1.0
			low = open - 1.0
		} else if step < 0 {
			high = open + 1.0
			low = close - 1.0
		}

		c := &ohlc.OHLC{
			Open:  open,
			High:  high,
			Low:   low,
			Close: close,
			Start: start.Add(time.Duration(i) * time.Hour),
			End:   start.Add(time.Duration(i+1) * time.Hour),
		}
		c.ForceClose()
		candles[i] = c
	}
	return candles
}

// generateSidewaysCandles creates oscillating OHLC candles to simulate a range-bound market (low ADX).
func generateSidewaysCandles(count int, basePrice float64) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < count; i++ {
		open := basePrice
		close := basePrice
		if i%2 == 0 {
			close += 1.0
		} else {
			close -= 1.0
		}

		c := &ohlc.OHLC{
			Open:  open,
			High:  basePrice + 2.0,
			Low:   basePrice - 2.0,
			Close: close,
			Start: start.Add(time.Duration(i) * time.Hour),
			End:   start.Add(time.Duration(i+1) * time.Hour),
		}
		c.ForceClose()
		candles[i] = c
	}
	return candles
}

func TestRSIADX_Name(t *testing.T) {
	t.Parallel()

	r := New("test", time.Minute*60)
	if strategy.NameRSIADX != r.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameRSIADX, r.Name())
	}
}

func TestRSIADX_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	r := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := r.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestRSIADX_Score(t *testing.T) {
	t.Parallel()

	t.Run("No WarmUp (Uninitialized Indicators)", func(t *testing.T) {
		r := New("test", time.Minute*60)
		candles := generateTestCandles(5, 100.0, 1.0)

		got := r.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when indicators are not ready", got)
		}
	})

	t.Run("Sideways Market (Weak ADX)", func(t *testing.T) {
		r := New("test", time.Minute*60)
		// Oscillating candles keep ADX below threshold (< 35)
		candles := generateSidewaysCandles(100, 100.0)

		for _, c := range candles {
			r.OnWarmUpCandle(c)
		}

		got := r.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when ADX is below threshold", got)
		}
	})

	t.Run("Strong Bullish Trend (Overbought / Negative Conviction)", func(t *testing.T) {
		r := New("test", time.Minute*60)
		// 100 candles rising steadily -> high ADX and overbought RSI
		candles := generateTestCandles(100, 100.0, 1.0)

		for _, c := range candles {
			r.OnWarmUpCandle(c)
		}

		got := r.Score(candles)
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 (SHORT signal / sell conviction)", got)
		}
	})

	t.Run("Strong Bearish Trend (Oversold / Positive Conviction)", func(t *testing.T) {
		r := New("test", time.Minute*60)
		// 100 candles falling steadily -> high ADX and oversold RSI
		candles := generateTestCandles(100, 500.0, -1.0)

		for _, c := range candles {
			r.OnWarmUpCandle(c)
		}

		got := r.Score(candles)
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 (LONG signal / buy conviction)", got)
		}
	})
}
