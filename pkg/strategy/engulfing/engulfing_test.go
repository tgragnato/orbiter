package engulfing

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

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
