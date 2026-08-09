package harami

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

func TestHarami_Name(t *testing.T) {
	t.Parallel()

	h := New("test", time.Minute*60)
	if strategy.NameHarami != h.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameHarami, h.Name())
	}
}

func TestHarami_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	h := New("test", time.Minute*60)
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := h.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}
