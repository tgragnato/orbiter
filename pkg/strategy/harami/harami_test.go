package harami

import (
	"testing"
	"time"

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

func TestHarami_Name(t *testing.T) {
	t.Parallel()

	h := New("test", time.Minute*60)
	if strategy.NameHarami != h.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameHarami, h.Name())
	}
}

func TestHarami_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	h := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := h.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestHarami_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	h := New("test", time.Minute*60)
	// CircularBuffer requires at least 7 candles to be populated
	candles := generateTestCandles(7, 100.0, 1.0)

	for _, c := range candles {
		h.OnWarmUpCandle(c)
	}

	maxLow, err := h.previousLows.Min()
	if err != nil {
		t.Fatalf("expected previousLows buffer to be populated, got error: %v", err)
	}
	if maxLow == 0 {
		t.Errorf("expected non-zero minimum low from buffer")
	}
}

func TestHarami_Score(t *testing.T) {
	t.Parallel()

	t.Run("Insufficient Candles", func(t *testing.T) {
		h := New("test", time.Minute*60)
		candles := generateTestCandles(1, 100.0, 1.0)

		got := h.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 for less than 2 candles", got)
		}
	})

	t.Run("Zero Previous Body", func(t *testing.T) {
		h := New("test", time.Minute*60)

		// Create previous candle with Open == Close (Doji / zero body)
		prev := &ohlc.OHLC{
			Open:  100.0,
			High:  101.0,
			Low:   99.0,
			Close: 100.0,
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		prev.ForceClose()

		current := &ohlc.OHLC{
			Open:  100.0,
			High:  102.0,
			Low:   99.5,
			Close: 101.5,
			Start: time.Now().Add(time.Hour),
			End:   time.Now().Add(time.Hour * 2),
		}
		current.ForceClose()

		got := h.Score([]*ohlc.OHLC{prev, current})
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when previous body is zero", got)
		}
	})

	t.Run("Bullish Harami (Positive Score)", func(t *testing.T) {
		h := New("test", time.Minute*60)

		// Large Bearish candle followed by small Bullish candle
		prev := &ohlc.OHLC{
			Open:  100.0,
			High:  101.0,
			Low:   79.0,
			Close: 80.0, // body = 20
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		prev.ForceClose()

		current := &ohlc.OHLC{
			Open:  82.0,
			High:  86.0,
			Low:   81.0,
			Close: 84.0, // body = 2
			Start: time.Now().Add(time.Hour),
			End:   time.Now().Add(time.Hour * 2),
		}
		current.ForceClose()

		got := h.Score([]*ohlc.OHLC{prev, current})
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 for Bullish Harami pattern", got)
		}
	})

	t.Run("Bearish Harami (Negative Score)", func(t *testing.T) {
		h := New("test", time.Minute*60)

		// Large Bullish candle followed by small Bearish candle
		prev := &ohlc.OHLC{
			Open:  80.0,
			High:  101.0,
			Low:   79.0,
			Close: 100.0, // body = 20
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		prev.ForceClose()

		current := &ohlc.OHLC{
			Open:  98.0,
			High:  99.0,
			Low:   95.0,
			Close: 96.0, // body = 2
			Start: time.Now().Add(time.Hour),
			End:   time.Now().Add(time.Hour * 2),
		}
		current.ForceClose()

		got := h.Score([]*ohlc.OHLC{prev, current})
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 for Bearish Harami pattern", got)
		}
	})
}
