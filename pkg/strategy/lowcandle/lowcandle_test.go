package lowcandle

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// generateTestCandles creates valid OHLC candles with ForceClose() applied.
func generateTestCandles(count int, basePrice float64) []*ohlc.OHLC {
	candles := make([]*ohlc.OHLC, count)
	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < count; i++ {
		// Alternate prices slightly to create a high/low range
		offset := float64(i % 3)
		open := basePrice + offset
		close := open + 0.5
		high := open + 2.0
		low := open - 2.0

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

func TestLowCandle_Name(t *testing.T) {
	t.Parallel()

	l := New("test", time.Minute*60)
	if strategy.NameLowCandle != l.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameLowCandle, l.Name())
	}
}

func TestLowCandle_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	l := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := l.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestLowCandle_Score_And_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	t.Run("No WarmUp (Empty Buffers)", func(t *testing.T) {
		l := New("test", time.Minute*60)
		candles := generateTestCandles(1, 100.0)

		got := l.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when circular buffers are uninitialized", got)
		}
	})

	t.Run("Inside Historical Range (Neutral Score)", func(t *testing.T) {
		l := New("test", time.Minute*60)
		warmUp := generateTestCandles(7, 100.0)

		for _, c := range warmUp {
			l.OnWarmUpCandle(c)
		}

		// Close price inside range
		testCandles := []*ohlc.OHLC{warmUp[0]}
		got := l.Score(testCandles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 for close price inside historical range", got)
		}
	})

	t.Run("Breakout Below Low (Positive Buy Score)", func(t *testing.T) {
		l := New("test", time.Minute*60)
		warmUp := generateTestCandles(7, 100.0)

		for _, c := range warmUp {
			l.OnWarmUpCandle(c)
		}

		// Historical Low is 98.0, High is 104.5 -> totalRange = 6.5
		// Create a candle with Close = 91.5 (below min low 98.0)
		breakoutCandle := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(93.0),
			High:  decimal.NewFromFloat(94.0),
			Low:   decimal.NewFromFloat(90.0),
			Close: decimal.NewFromFloat(91.5),
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		breakoutCandle.ForceClose()

		got := l.Score([]*ohlc.OHLC{breakoutCandle})
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 for breakout below historical low", got)
		}
	})

	t.Run("Breakout Above High (Negative Sell Score)", func(t *testing.T) {
		l := New("test", time.Minute*60)
		warmUp := generateTestCandles(7, 100.0)

		for _, c := range warmUp {
			l.OnWarmUpCandle(c)
		}

		// Create a candle with Close = 110.0 (above max high 104.5)
		breakoutCandle := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(108.0),
			High:  decimal.NewFromFloat(111.0),
			Low:   decimal.NewFromFloat(107.0),
			Close: decimal.NewFromFloat(110.0),
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		breakoutCandle.ForceClose()

		got := l.Score([]*ohlc.OHLC{breakoutCandle})
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 for breakout above historical high", got)
		}
	})
}
