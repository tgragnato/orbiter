package doji

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

func TestDoji_Name(t *testing.T) {
	t.Parallel()

	d := New("test")
	if strategy.NameDOJI != d.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameDOJI, d.Name())
	}
}

func TestDoji_OnWarmUpCandle(t *testing.T) {
	t.Parallel()

	d := New("test")
	c := ohlc.New("test", time.Now(), time.Minute*60, false)
	c.NewPrice(decimal.NewFromFloat(100), c.Start)
	c.ForceClose()

	d.OnWarmUpCandle(c)
	if d.previousCandle != c {
		t.Errorf("expected previousCandle to be updated by OnWarmUpCandle")
	}
}

func TestDoji_OnTick_Long(t *testing.T) {
	t.Parallel()

	d := New("test")
	// Mock a DOJI candle
	prevCandle := ohlc.New("test", time.Now(), time.Minute*60, false)
	prevCandle.NewPrice(decimal.NewFromFloat(100), prevCandle.Start)
	prevCandle.NewPrice(decimal.NewFromFloat(100.01), prevCandle.Start)
	prevCandle.ForceClose()
	// High: 100.01, Low: 100

	d.OnCandle([]*ohlc.OHLC{prevCandle})

	// Tick breaks high by > 2 pips (0.0002)
	// 100.01 + 0.0002 = 100.0102
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100.0103), decimal.NewFromFloat(100.0103))

	toOpen, _, _ := d.OnTick(currentTick)
	if len(toOpen) != 1 {
		t.Fatalf("expected 1 order, got %d", len(toOpen))
	}
	if toOpen[0].Direction != broker.BuyDirectionLong {
		t.Fatalf("expected BuyDirectionLong, got %v", toOpen[0].Direction)
	}
}

func TestDoji_OnTick_Short(t *testing.T) {
	t.Parallel()

	d := New("test")
	// Mock a DOJI candle
	prevCandle := ohlc.New("test", time.Now(), time.Minute*60, false)
	prevCandle.NewPrice(decimal.NewFromFloat(100), prevCandle.Start)
	prevCandle.NewPrice(decimal.NewFromFloat(100.01), prevCandle.Start)
	prevCandle.ForceClose()
	// High: 100.01, Low: 100

	d.OnCandle([]*ohlc.OHLC{prevCandle})

	// Tick breaks low by > 2 pips (0.0002)
	// 100 - 0.0002 = 99.9998
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(99.9997), decimal.NewFromFloat(99.9997))

	toOpen, _, _ := d.OnTick(currentTick)
	if len(toOpen) != 1 {
		t.Fatalf("expected 1 order, got %d", len(toOpen))
	}
	if toOpen[0].Direction != broker.BuyDirectionShort {
		t.Fatalf("expected BuyDirectionShort, got %v", toOpen[0].Direction)
	}
}

func TestDoji_Score(t *testing.T) {
	t.Parallel()

	t.Run("Nil Previous Candle", func(t *testing.T) {
		d := New("test")
		got := d.Score(nil)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when previousCandle is nil", got)
		}
	})

	t.Run("DOJI Candle Active", func(t *testing.T) {
		d := New("test")
		c := ohlc.New("test", time.Now(), time.Minute*60, false)
		c.NewPrice(decimal.NewFromFloat(100), c.Start)
		c.NewPrice(decimal.NewFromFloat(100.01), c.Start)
		c.ForceClose()

		d.OnWarmUpCandle(c)

		got := d.Score(nil)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when previousCandle is DOJI", got)
		}
	})

	t.Run("Non-DOJI Normal Range", func(t *testing.T) {
		d := New("test")
		c := ohlc.New("test", time.Now(), time.Minute*60, false)
		c.NewPrice(decimal.NewFromFloat(100), c.Start)
		c.NewPrice(decimal.NewFromFloat(105), c.Start) // 5% performance, not DOJI
		c.ForceClose()

		d.OnWarmUpCandle(c)

		got := d.Score(nil)
		if got != 0.0 {
			t.Errorf("Score() = %v, want 0.0 when close is within range", got)
		}
	})

	t.Run("Close Above High Breakout", func(t *testing.T) {
		d := New("test")
		c := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(100.0),
			High:  decimal.NewFromFloat(102.0),
			Low:   decimal.NewFromFloat(99.0),
			Close: decimal.NewFromFloat(103.0), // Close > High
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		c.ForceClose()

		d.OnWarmUpCandle(c)

		got := d.Score(nil)
		if got <= 0.0 {
			t.Errorf("Score() = %v, want > 0.0 when Close > High", got)
		}
	})

	t.Run("Close Below Low Breakout", func(t *testing.T) {
		d := New("test")
		c := &ohlc.OHLC{
			Open:  decimal.NewFromFloat(100.0),
			High:  decimal.NewFromFloat(101.0),
			Low:   decimal.NewFromFloat(98.0),
			Close: decimal.NewFromFloat(97.0), // Close < Low
			Start: time.Now(),
			End:   time.Now().Add(time.Hour),
		}
		c.ForceClose()

		d.OnWarmUpCandle(c)

		got := d.Score(nil)
		if got >= 0.0 {
			t.Errorf("Score() = %v, want < 0.0 when Close < Low", got)
		}
	})
}
