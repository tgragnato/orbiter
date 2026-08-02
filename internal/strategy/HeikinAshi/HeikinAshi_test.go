package heikinashi

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/internal/broker"
	"github.com/tgragnato/orbiter/internal/strategy"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func TestHeikinAshi_Name(t *testing.T) {
	t.Parallel()

	ha := New("test")
	if strategy.NameHeikinAshi != ha.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameHeikinAshi, ha.Name())
	}
}

func getHACandlesLong(amount int) (candles []*ohlc.OHLC) {
	for i := 0; i < amount; i++ {
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
	for i := 0; i < amount; i++ {
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
