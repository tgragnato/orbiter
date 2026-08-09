package lowcandle

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

func TestLowCandle_Name(t *testing.T) {
	t.Parallel()

	l := New("test", time.Minute*60)
	if strategy.NameLowCandle != l.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameLowCandle, l.Name())
	}
}

func TestLowCandle_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	l := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := l.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}
