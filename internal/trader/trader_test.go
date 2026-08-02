package trader

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sklinkert/at/pkg/tick"
)

func TestTrader_flashCrashCheck(t *testing.T) {
	t.Parallel()

	tick1 := tick.New("", time.Now(), decimal.NewFromFloat(100.0), decimal.NewFromFloat(100.0))
	tick2 := tick.New("", time.Now(), decimal.NewFromFloat(100.1), decimal.NewFromFloat(100.1))
	if flashCrashCheck(tick1, tick2) != nil {
		t.Fatalf("unexpected error: %v", flashCrashCheck(tick1, tick2))
	}

	tick1 = tick.New("", time.Now(), decimal.NewFromFloat(1.0), decimal.NewFromFloat(1.00))
	tick2 = tick.New("", time.Now(), decimal.NewFromFloat(2.0), decimal.NewFromFloat(2.00))
	if !(flashCrashCheck(tick1, tick2) != nil) {
		t.Fatalf("expected true")
	}
}

func Test__distanceInPercentage(t *testing.T) {
	t.Parallel()

	price1 := decimal.NewFromFloat(10)
	price2 := decimal.NewFromFloat(12)
	if "20" != distanceInPercentage(price1, price2).String() {
		t.Fatalf("expected %q, got %q", "20", distanceInPercentage(price1, price2).String())
	}

	price1 = decimal.NewFromFloat(10)
	price2 = decimal.NewFromFloat(8)
	if "-20" != distanceInPercentage(price1, price2).String() {
		t.Fatalf("expected %q, got %q", "-20", distanceInPercentage(price1, price2).String())
	}

	price1 = decimal.NewFromFloat(1)
	price2 = decimal.NewFromFloat(2)
	if "100" != distanceInPercentage(price1, price2).String() {
		t.Fatalf("expected %q, got %q", "100", distanceInPercentage(price1, price2).String())
	}
}
