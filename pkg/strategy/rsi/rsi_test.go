package rsi

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// generateTestCandles creates valid OHLC candles and calls ForceClose() on each.
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
			Open:  decimal.NewFromFloat(open),
			High:  decimal.NewFromFloat(high),
			Low:   decimal.NewFromFloat(low),
			Close: decimal.NewFromFloat(close),
			Start: start.Add(time.Duration(i) * time.Hour),
			End:   start.Add(time.Duration(i+1) * time.Hour),
		}
		c.ForceClose()
		candles[i] = c
	}
	return candles
}

func TestRSI_Name(t *testing.T) {
	t.Parallel()

	r := New("test", time.Minute*60)
	if strategy.NameRSI != r.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameRSI, r.Name())
	}
}

func TestRSI_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	r := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := r.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestRSI_Score_And_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	t.Run("No WarmUp (Uninitialized Indicators)", func(t *testing.T) {
		r := New("test", time.Minute*60)
		candles := generateTestCandles(5, 100.0, 1.0)

		got := r.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when RSI is uninitialized", got)
		}
	})

	t.Run("WarmUp with Bullish Trend (Overbought / Negative Conviction)", func(t *testing.T) {
		r := New("test", time.Minute*60)
		candles := generateTestCandles(50, 100.0, 2.0)

		for _, c := range candles {
			r.OnWarmUpCandle(c)
		}

		got := r.Score(candles)
		if got != -1.0 {
			t.Errorf("Score() = %v, want -1.0 (overbought RSI)", got)
		}
	})

	t.Run("WarmUp with Bearish Trend (Oversold / Positive Conviction)", func(t *testing.T) {
		r := New("test", time.Minute*60)
		candles := generateTestCandles(50, 500.0, -2.0)

		for _, c := range candles {
			r.OnWarmUpCandle(c)
		}

		got := r.Score(candles)
		if got != 1.0 {
			t.Errorf("Score() = %v, want 1.0 (oversold RSI)", got)
		}
	})
}
