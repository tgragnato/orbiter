package rsi

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

func TestRSI_Name(t *testing.T) {
	t.Parallel()

	r := New("test", time.Minute*60)
	if strategy.NameRSI != r.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameRSI, r.Name())
	}
}

func TestRSI_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	r := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := r.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}
