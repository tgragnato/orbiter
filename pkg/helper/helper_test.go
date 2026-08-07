package helper

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/broker"
)

func TestCalcStopLossPriceByPercentage(t *testing.T) {
	t.Parallel()

	price := decimal.NewFromFloat(100)
	stopLossPercentage := decimal.NewFromFloat(20)

	// Long
	stopPrice := CalcStopLossPriceByPercentage(price, stopLossPercentage, broker.BuyDirectionLong)
	stopPriceFloat, _ := stopPrice.Float64()
	if stopPriceFloat != 80 {
		t.Fatalf("expected %v, got %v", 80, stopPriceFloat)
	}

	// Short
	stopPrice = CalcStopLossPriceByPercentage(price, stopLossPercentage, broker.BuyDirectionShort)
	stopPriceFloat, _ = stopPrice.Float64()
	if stopPriceFloat != 120 {
		t.Fatalf("expected %v, got %v", 120, stopPriceFloat)
	}
}

func TestTargetPriceByPercentage(t *testing.T) {
	t.Parallel()

	price := decimal.NewFromFloat(100)
	targetPercentage := decimal.NewFromFloat(20)

	// Long
	targetPrice := CalcTargetPriceByPercentage(price, targetPercentage, broker.BuyDirectionLong)
	targetPriceFloat, _ := targetPrice.Float64()
	if targetPriceFloat != 120 {
		t.Fatalf("expected %v, got %v", 120, targetPriceFloat)
	}

	// Short
	targetPrice = CalcTargetPriceByPercentage(price, targetPercentage, broker.BuyDirectionShort)
	targetPriceFloat, _ = targetPrice.Float64()
	if targetPriceFloat != 80 {
		t.Fatalf("expected %v, got %v", 80, targetPriceFloat)
	}
}

func TestDecimalToFloat(t *testing.T) {
	t.Parallel()

	n := decimal.NewFromFloat(10.34)
	if DecimalToFloat(n) != 10.34 {
		t.Fatalf("expected %v, got %v", 10.34, DecimalToFloat(n))
	}
}

func TestFloatToDecimal(t *testing.T) {
	t.Parallel()

	n := 10.34
	want := decimal.NewFromFloat(n)
	if !(want.Equal(FloatToDecimal(n))) {
		t.Fatalf("expected true")
	}
}
