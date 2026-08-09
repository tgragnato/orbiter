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
