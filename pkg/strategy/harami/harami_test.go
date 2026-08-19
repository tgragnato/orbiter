//nolint:testpackage // accesses unexported field previousLows
package harami

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
		closeVal := open + step
		high := open + 1.0
		low := open - 1.0

		if step > 0 {
			high = closeVal + 1.0
			low = open - 1.0
		} else if step < 0 {
			high = open + 1.0
			low = closeVal - 1.0
		}

		candle := &ohlc.OHLC{
			Instrument: "",
			Open:       open,
			High:       high,
			HighTime:   time.Time{},
			Low:        low,
			LowTime:    time.Time{},
			Close:      closeVal,
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

func TestHarami_Name(t *testing.T) {
	t.Parallel()

	harami := New("test", time.Minute*60)
	if strategy.NameHarami != harami.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameHarami, harami.Name())
	}
}

func TestHarami_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	harami := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), 100, 100)

	toOpen, _, _ := harami.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestHarami_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	harami := New("test", time.Minute*60)
	// CircularBuffer requires at least 7 candles to be populated
	candles := generateTestCandles(7, 100.0, 1.0)

	for _, c := range candles {
		harami.OnWarmUpCandle(c)
	}

	maxLow, err := harami.previousLows.Min()
	if err != nil {
		t.Fatalf("expected previousLows buffer to be populated, got error: %v", err)
	}

	if maxLow == 0 {
		t.Errorf("expected non-zero minimum low from buffer")
	}
}

//nolint:funlen // table-driven subtests require length
func TestHarami_Score(t *testing.T) {
	t.Parallel()

	t.Run("Insufficient Candles", func(t *testing.T) {
		t.Parallel()

		harami := New("test", time.Minute*60)
		candles := generateTestCandles(1, 100.0, 1.0)

		got := harami.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 for less than 2 candles", got)
		}
	})

	t.Run("Zero Previous Body", func(t *testing.T) { //nolint:dupl // distinct test case despite structural similarity
		t.Parallel()

		harami := New("test", time.Minute*60)

		// Create previous candle with Open == Close (Doji / zero body)
		prev := &ohlc.OHLC{
			Instrument: "",
			Open:       100.0,
			High:       101.0,
			HighTime:   time.Time{},
			Low:        99.0,
			LowTime:    time.Time{},
			Close:      100.0,
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		prev.ForceClose()

		current := &ohlc.OHLC{
			Instrument: "",
			Open:       100.0,
			High:       102.0,
			HighTime:   time.Time{},
			Low:        99.5,
			LowTime:    time.Time{},
			Close:      101.5,
			Start:      time.Now().Add(time.Hour),
			End:        time.Now().Add(time.Hour * 2),
			Duration:   0,
			Gaps:       false,
		}
		current.ForceClose()

		got := harami.Score([]*ohlc.OHLC{prev, current})
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when previous body is zero", got)
		}
	})

	t.Run("Bullish Harami (Positive Score)", func(t *testing.T) { //nolint:dupl // distinct scenario
		t.Parallel()

		harami := New("test", time.Minute*60)

		// Large Bearish candle followed by small Bullish candle
		prev := &ohlc.OHLC{
			Instrument: "",
			Open:       100.0,
			High:       101.0,
			HighTime:   time.Time{},
			Low:        79.0,
			LowTime:    time.Time{},
			Close:      80.0, // body = 20
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		prev.ForceClose()

		current := &ohlc.OHLC{
			Instrument: "",
			Open:       82.0,
			High:       86.0,
			HighTime:   time.Time{},
			Low:        81.0,
			LowTime:    time.Time{},
			Close:      84.0, // body = 2
			Start:      time.Now().Add(time.Hour),
			End:        time.Now().Add(time.Hour * 2),
			Duration:   0,
			Gaps:       false,
		}
		current.ForceClose()

		got := harami.Score([]*ohlc.OHLC{prev, current})
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 for Bullish Harami pattern", got)
		}
	})

	t.Run("Bearish Harami (Negative Score)", func(t *testing.T) { //nolint:dupl // distinct scenario
		t.Parallel()

		harami := New("test", time.Minute*60)

		// Large Bullish candle followed by small Bearish candle
		prev := &ohlc.OHLC{
			Instrument: "",
			Open:       80.0,
			High:       101.0,
			HighTime:   time.Time{},
			Low:        79.0,
			LowTime:    time.Time{},
			Close:      100.0, // body = 20
			Start:      time.Now(),
			End:        time.Now().Add(time.Hour),
			Duration:   0,
			Gaps:       false,
		}
		prev.ForceClose()

		current := &ohlc.OHLC{
			Instrument: "",
			Open:       98.0,
			High:       99.0,
			HighTime:   time.Time{},
			Low:        95.0,
			LowTime:    time.Time{},
			Close:      96.0, // body = 2
			Start:      time.Now().Add(time.Hour),
			End:        time.Now().Add(time.Hour * 2),
			Duration:   0,
			Gaps:       false,
		}
		current.ForceClose()

		got := harami.Score([]*ohlc.OHLC{prev, current})
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 for Bearish Harami pattern", got)
		}
	})
}
