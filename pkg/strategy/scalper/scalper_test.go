package scalper

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

func TestScalper_Name(t *testing.T) {
	t.Parallel()

	s := New("test")
	if strategy.NameScalper != s.Name() {
		t.Fatalf("expected %q, got %q", strategy.NameScalper, s.Name())
	}
}

func TestScalper_OnTick_NoOrders(t *testing.T) {
	t.Parallel()

	s := New("test")
	currentTick := tick.New("test", time.Now(), decimal.NewFromFloat(100), decimal.NewFromFloat(100))

	toOpen, _, _ := s.OnTick(currentTick)
	if len(toOpen) != 0 {
		t.Fatalf("expected 0 orders, got %d", len(toOpen))
	}
}
