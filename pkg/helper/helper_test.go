package helper

import (
	"testing"

	"github.com/tgragnato/orbiter/pkg/broker"
)

func TestCalcStopLossPriceByPercentage(t *testing.T) {
	t.Parallel()

	price := 100.0
	stopLossPercentage := 20.0

	// Long
	stopPrice := CalcStopLossPriceByPercentage(price, stopLossPercentage, broker.BuyDirectionLong)
	if stopPrice != 80 {
		t.Fatalf("expected %v, got %v", 80, stopPrice)
	}

	// Short
	stopPrice = CalcStopLossPriceByPercentage(price, stopLossPercentage, broker.BuyDirectionShort)
	if stopPrice != 120 {
		t.Fatalf("expected %v, got %v", 120, stopPrice)
	}
}

func TestTargetPriceByPercentage(t *testing.T) {
	t.Parallel()

	price := 100.0
	targetPercentage := 20.0

	// Long
	targetPrice := CalcTargetPriceByPercentage(price, targetPercentage, broker.BuyDirectionLong)
	if targetPrice != 120 {
		t.Fatalf("expected %v, got %v", 120, targetPrice)
	}

	// Short
	targetPrice = CalcTargetPriceByPercentage(price, targetPercentage, broker.BuyDirectionShort)
	if targetPrice != 80 {
		t.Fatalf("expected %v, got %v", 80, targetPrice)
	}
}
