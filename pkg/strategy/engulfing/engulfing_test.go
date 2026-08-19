//nolint:testpackage // accesses unexported field sma for warm-up verification
package engulfing

import (
	"testing"
	"time"

	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// generateTestCandles creates valid OHLC candles with ForceClose() applied.
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

func TestEngulfing_Name(t *testing.T) {
	t.Parallel()

	eng := New("test", time.Minute*60)
	if strategy.NameEngulfing != eng.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameEngulfing, eng.Name())
	}
}

func TestEngulfing_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	eng := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := eng.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestEngulfing_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	eng := New("test", time.Minute*60)
	// SMA indicator requires 200 candles to produce a value
	candles := generateTestCandles(200, 100.0, 0.5)

	for _, c := range candles {
		eng.OnWarmUpCandle(c)
	}

	smaVal, err := eng.sma.Value()
	if err != nil {
		t.Fatalf("expected SMA indicator to be initialized, got error: %v", err)
	}

	if len(smaVal) == 0 {
		t.Errorf("expected non-empty SMA value map")
	}
}

//nolint:funlen // test function covers multiple scenarios
func TestEngulfing_Score(t *testing.T) {
	t.Parallel()

	t.Run("Insufficient Candles", func(t *testing.T) {
		t.Parallel()

		eng := New("test", time.Minute*60)
		candles := generateTestCandles(1, 100.0, 1.0)

		got := eng.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 for less than 2 candles", got)
		}
	})

	//nolint:dupl // subtests share similar structure but test distinct scenarios
	t.Run("Zero Previous Body", func(t *testing.T) {
		t.Parallel()

		eng := New("test", time.Minute*60)

		prev := &ohlc.OHLC{
			Instrument: "",
			Open:       100.0,
			High:       101.0,
			HighTime:   time.Time{},
			Low:        99.0,
			LowTime:    time.Time{},
			Close:      100.0, // Zero body (Doji)
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		prev.ForceClose()

		current := &ohlc.OHLC{
			Instrument: "",
			Open:       101.0,
			High:       102.0,
			HighTime:   time.Time{},
			Low:        98.0,
			LowTime:    time.Time{},
			Close:      99.0,
			Start:      time.Now().Add(time.Hour),
			End:        time.Now().Add(time.Hour * 2),
			Duration:   0,
			Gaps:       false,
		}
		current.ForceClose()

		got := eng.Score([]*ohlc.OHLC{prev, current})
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when previous body size is zero", got)
		}
	})

	//nolint:dupl // subtests share similar structure but test distinct scenarios
	t.Run("Bearish Engulfing (Positive Score)", func(t *testing.T) {
		t.Parallel()

		eng := New("test", time.Minute*60)

		// Previous candle: Bullish (Open: 90, Close: 100) -> Body: 10
		prev := &ohlc.OHLC{
			Instrument: "",
			Open:       90.0,
			High:       101.0,
			HighTime:   time.Time{},
			Low:        89.0,
			LowTime:    time.Time{},
			Close:      100.0,
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		prev.ForceClose()

		// Current candle: Bearish (Open: 101, Close: 89) -> Body: 12
		curr := &ohlc.OHLC{
			Instrument: "",
			Open:       101.0,
			High:       102.0,
			HighTime:   time.Time{},
			Low:        88.0,
			LowTime:    time.Time{},
			Close:      89.0,
			Start:      time.Now().Add(time.Hour),
			End:        time.Now().Add(time.Hour * 2),
			Duration:   0,
			Gaps:       false,
		}
		curr.ForceClose()

		got := eng.Score([]*ohlc.OHLC{prev, curr})
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 for Bearish Engulfing pattern", got)
		}
	})

	//nolint:dupl // subtests share similar structure but test distinct scenarios
	t.Run("Bullish Engulfing (Negative Score)", func(t *testing.T) {
		t.Parallel()

		eng := New("test", time.Minute*60)

		// Previous candle: Bearish (Open: 100, Close: 90) -> Body: 10
		prev := &ohlc.OHLC{
			Instrument: "",
			Open:       100.0,
			High:       101.0,
			HighTime:   time.Time{},
			Low:        89.0,
			LowTime:    time.Time{},
			Close:      90.0,
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		prev.ForceClose()

		// Current candle: Bullish (Open: 91, Close: 98) -> Body: 7
		curr := &ohlc.OHLC{
			Instrument: "",
			Open:       91.0,
			High:       99.0,
			HighTime:   time.Time{},
			Low:        90.0,
			LowTime:    time.Time{},
			Close:      98.0,
			Start:      time.Now().Add(time.Hour),
			End:        time.Now().Add(time.Hour * 2),
			Duration:   0,
			Gaps:       false,
		}
		curr.ForceClose()

		got := eng.Score([]*ohlc.OHLC{prev, curr})
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 for Bullish Engulfing pattern", got)
		}
	})

	t.Run("No Engulfing Pattern Present", func(t *testing.T) {
		t.Parallel()

		eng := New("test", time.Minute*60)
		candles := generateTestCandles(2, 100.0, 1.0) // Standard rising candles

		got := eng.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when no engulfing pattern is present", got)
		}
	})
}
