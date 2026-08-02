package tick

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestTick_Spread(t *testing.T) {
	t.Parallel()

	var bid = decimal.NewFromFloat(1.00)
	var ask = decimal.NewFromFloat(1.50)
	var tick = New("EURUSD", time.Now(), bid, ask)
	var spread, _ = tick.Spread().Float64()
	if 0.50 != spread {
		t.Fatalf("expected %v, got %v", 0.50, spread)
	}
}

func TestTick_SpreadInPercent(t *testing.T) {
	t.Parallel()

	var bid = decimal.NewFromFloat(0.80)
	var ask = decimal.NewFromFloat(1.50)
	var tick = New("EURUSD", time.Now(), bid, ask)
	var spread, _ = tick.SpreadInPercent().Float64()
	if 87.50 != spread {
		t.Fatalf("expected %v, got %v", 87.50, spread)
	}
}
