package engulfing

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// generateTestCandles creates valid OHLC candles with ForceClose() applied.
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

func TestEngulfing_Name(t *testing.T) {
	t.Parallel()

	e := New("test", time.Minute*60)
	if strategy.NameEngulfing != e.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameEngulfing, e.Name())
	}
}

func TestEngulfing_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	e := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := e.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestEngulfing_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	e := New("test", time.Minute*60)
	// SMA indicator requires 200 candles to produce a value
	candles := generateTestCandles(200, 100.0, 0.5)

	for _, c := range candles {
		e.OnWarmUpCandle(c)
	}

	smaVal, err := e.sma.Value()
	if err != nil {
		t.Fatalf("expected SMA indicator to be initialized, got error: %v", err)
	}
	if len(smaVal) == 0 {
		t.Errorf("expected non-empty SMA value map")
	}
}

func TestEngulfing_Score(t *testing.T) {
	t.Parallel()

	t.Run("Insufficient Candles", func(t *testing.T) {
		e := New("test", time.Minute*60)
		candles := generateTestCandles(1, 100.0, 1.0)

		got := e.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 for less than 2 candles", got)
		}
	})

	t.Run("Zero Previous Body", func(t *testing.T) {
		e := New("test", time.Minute*60)

		prev := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(100.0),
			High:  decimal.NewFromFloat(101.0),
			Low:   decimal.NewFromFloat(99.0),
			Close: decimal.NewFromFloat(100.0), // Zero body (Doji)
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		prev.ForceClose()

		current := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(101.0),
			High:  decimal.NewFromFloat(102.0),
			Low:   decimal.NewFromFloat(98.0),
			Close: decimal.NewFromFloat(99.0),
			Start: time.Now().Add(time.Hour),
			End:   time.Now().Add(time.Hour * 2),
		}
		current.ForceClose()

		got := e.Score([]*ohlc.OHLC{prev, current})
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when previous body size is zero", got)
		}
	})

	t.Run("Bearish Engulfing (Positive Score)", func(t *testing.T) {
		e := New("test", time.Minute*60)

		// Previous candle: Bullish (Open: 90, Close: 100) -> Body: 10
		prev := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(90.0),
			High:  decimal.NewFromFloat(101.0),
			Low:   decimal.NewFromFloat(89.0),
			Close: decimal.NewFromFloat(100.0),
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		prev.ForceClose()

		// Current candle: Bearish (Open: 101, Close: 89) -> Body: 12
		curr := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(101.0),
			High:  decimal.NewFromFloat(102.0),
			Low:   decimal.NewFromFloat(88.0),
			Close: decimal.NewFromFloat(89.0),
			Start: time.Now().Add(time.Hour),
			End:   time.Now().Add(time.Hour * 2),
		}
		curr.ForceClose()

		got := e.Score([]*ohlc.OHLC{prev, curr})
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 for Bearish Engulfing pattern", got)
		}
	})

	t.Run("Bullish Engulfing (Negative Score)", func(t *testing.T) {
		e := New("test", time.Minute*60)

		// Previous candle: Bearish (Open: 100, Close: 90) -> Body: 10
		prev := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(100.0),
			High:  decimal.NewFromFloat(101.0),
			Low:   decimal.NewFromFloat(89.0),
			Close: decimal.NewFromFloat(90.0),
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		prev.ForceClose()

		// Current candle: Bullish (Open: 91, Close: 98) -> Body: 7
		curr := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(91.0),
			High:  decimal.NewFromFloat(99.0),
			Low:   decimal.NewFromFloat(90.0),
			Close: decimal.NewFromFloat(98.0),
			Start: time.Now().Add(time.Hour),
			End:   time.Now().Add(time.Hour * 2),
		}
		curr.ForceClose()

		got := e.Score([]*ohlc.OHLC{prev, curr})
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 for Bullish Engulfing pattern", got)
		}
	})

	t.Run("No Engulfing Pattern Present", func(t *testing.T) {
		e := New("test", time.Minute*60)
		candles := generateTestCandles(2, 100.0, 1.0) // Standard rising candles

		got := e.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when no engulfing pattern is present", got)
		}
	})
}
