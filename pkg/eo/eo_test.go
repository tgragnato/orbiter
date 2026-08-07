package eo

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

func Test_riskLevelHigh(t *testing.T) {
	t.Parallel()

	overlay := New()

	for i := range 100 {
		candle := generateCandle(float64(i))
		overlay.AddCandle(candle)
	}
	level := overlay.riskLevel()
	if int(RExtreme) != int(level) {
		t.Fatalf("expected %d, got %d", int(RExtreme), int(level))
	}
}

func Test_riskLevelLow(t *testing.T) {
	t.Parallel()

	overlay := New()

	for i := 100; i > 0; i-- {
		candle := generateCandle(float64(i))
		overlay.AddCandle(candle)
	}
	level := overlay.riskLevel()
	if int(RLow) != int(level) {
		t.Fatalf("expected %d, got %d", int(RLow), int(level))
	}
}

func generateCandle(diff float64) *ohlc.OHLC {
	now := time.Now()
	candle := ohlc.New("test", now, time.Minute, false)
	candle.NewPrice(decimal.NewFromFloat(10), now)
	candle.NewPrice(decimal.NewFromFloat(10+diff), now)
	candle.ForceClose()
	return candle
}
