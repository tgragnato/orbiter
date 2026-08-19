package rsi_test

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/strategy/rsi"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// generateTestCandles creates valid OHLC candles and calls ForceClose() on each.
func generateTestCandles(count int, startPrice, step float64) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	for idx := range count {
		openPrice := startPrice + float64(idx)*step
		closePrice := openPrice + step
		high := openPrice + 1.0
		low := openPrice - 1.0

		if step > 0 {
			high = closePrice + 1.0
			low = openPrice - 1.0
		} else if step < 0 {
			high = openPrice + 1.0
			low = closePrice - 1.0
		}

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

func TestRSI_Name(t *testing.T) {
	t.Parallel()

	rsiStrategy := rsi.New("test", time.Minute*60)
	if strategy.NameRSI != rsiStrategy.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameRSI, rsiStrategy.Name())
	}
}

func TestRSI_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	rsiStrategy := rsi.New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := rsiStrategy.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestRSI_Score_And_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	t.Run("No WarmUp (Uninitialized Indicators)", func(t *testing.T) {
		t.Parallel()

		rsiStrategy := rsi.New("test", time.Minute*60)
		candles := generateTestCandles(5, 100.0, 1.0)

		got := rsiStrategy.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when RSI is uninitialized", got)
		}
	})

	t.Run("WarmUp with Bullish Trend (Overbought / Negative Conviction)", func(t *testing.T) {
		t.Parallel()

		rsiStrategy := rsi.New("test", time.Minute*60)
		candles := generateTestCandles(50, 100.0, 2.0)

		for _, c := range candles {
			rsiStrategy.OnWarmUpCandle(c)
		}

		got := rsiStrategy.Score(candles)
		if got != -1.0 {
			t.Errorf("Score() = %v, want -1.0 (overbought RSI)", got)
		}
	})

	t.Run("WarmUp with Bearish Trend (Oversold / Positive Conviction)", func(t *testing.T) {
		t.Parallel()

		rsiStrategy := rsi.New("test", time.Minute*60)
		candles := generateTestCandles(50, 500.0, -2.0)

		for _, c := range candles {
			rsiStrategy.OnWarmUpCandle(c)
		}

		got := rsiStrategy.Score(candles)
		if got != 1.0 {
			t.Errorf("Score() = %v, want 1.0 (oversold RSI)", got)
		}
	})
}
