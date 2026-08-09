package heikinashi

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/broker"
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

func TestHeikinAshi_Name(t *testing.T) {
	t.Parallel()

	ha := New("test")
	if strategy.NameHeikinAshi != ha.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameHeikinAshi, ha.Name())
	}
}

func getHACandlesLong(amount int) (candles []*ohlc.OHLC) {
	for range amount {
		now := time.Now()
		o := ohlc.New("test", now, time.Minute, false)
		o.NewPrice(decimal.NewFromFloat(float64(1)), o.Start)
		o.NewPrice(decimal.NewFromFloat(float64(2)), o.Start)
		o.ForceClose()
		candles = append(candles, o)
	}
	return candles
}

func getHACandlesShort(amount int) (candles []*ohlc.OHLC) {
	for range amount {
		now := time.Now()
		o := ohlc.New("test", now, time.Minute, false)
		o.NewPrice(decimal.NewFromFloat(float64(2)), o.Start)
		o.NewPrice(decimal.NewFromFloat(float64(1)), o.Start)
		o.ForceClose()
		candles = append(candles, o)
	}
	return candles
}

func TestHeikinAshi_checkCandleAmount(t *testing.T) {
	t.Parallel()

	ha := New("test")
	err := ha.checkCandleAmount(broker.BuyDirectionLong, 0)
	if err ==
		nil || !strings.Contains(err.
		Error(), "not enough closed candles to check",
	) {
		t.Fatalf("expected error containing not enough closed candles to check, got %v", err)
	}

	// All candles in the wrong direction
	ha.closedHACandles = getHACandlesShort(6)
	err = ha.checkCandleAmount(broker.BuyDirectionLong, 0)
	if err ==
		nil || !strings.Contains(err.
		Error(), "not enough candles in the right direction",
	) {
		t.Fatalf("expected error containing not enough candles in the right direction, got %v", err)
	}

	// All candles in the wrong direction with offset
	ha.closedHACandles = getHACandlesShort(6)
	err = ha.checkCandleAmount(broker.BuyDirectionLong, 2)
	if err ==
		nil || !strings.Contains(err.
		Error(), "not enough candles in the right direction",
	) {
		t.Fatalf("expected error containing not enough candles in the right direction, got %v", err)
	}

	// All candles in the right direction with offset
	ha.closedHACandles = getHACandlesLong(4)
	ha.closedHACandles = append(ha.closedHACandles, getHACandlesShort(2)...)
	err = ha.checkCandleAmount(broker.BuyDirectionLong, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHeikinAshi_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	ha := New("test")
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := ha.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}

func TestHeikinAshi_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	ha := New("test")
	candles := generateTestCandles(5, 100.0, 1.0)

	for _, c := range candles {
		ha.OnWarmUpCandle(c)
	}

	if !ha.candlesReceived {
		t.Errorf("expected candlesReceived to be true after warm-up")
	}
}

func TestHeikinAshi_Score(t *testing.T) {
	t.Parallel()

	t.Run("Insufficient Candles", func(t *testing.T) {
		ha := New("test")
		candles := generateTestCandles(2, 100.0, 1.0)

		got := ha.Score(candles)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 for less than 3 candles", got)
		}
	})

	t.Run("Bullish Trend (Positive Score)", func(t *testing.T) {
		ha := New("test")
		candles := generateTestCandles(4, 100.0, 2.0)

		got := ha.Score(candles)
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 for bullish Heikin-Ashi candles", got)
		}
	})

	t.Run("Bearish Trend (Negative Score)", func(t *testing.T) {
		ha := New("test")
		candles := generateTestCandles(4, 200.0, -2.0)

		got := ha.Score(candles)
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 for bearish Heikin-Ashi candles", got)
		}
	})
}
