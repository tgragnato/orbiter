package broker

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestPerformanceInPercentage(t *testing.T) {
	t.Parallel()

	currentPrice := decimal.NewFromFloat(2.0)

	// Long
	position := Position{
		BuyPrice:     decimal.NewFromFloat(1.0),
		BuyDirection: BuyDirectionLong,
	}
	perf := position.PerformanceInPercentage(currentPrice, currentPrice)
	if perf != 100 {
		t.Fatalf("expected %v, got %v", 100, perf)
	}

	// Short
	currentPrice = decimal.NewFromFloat(1.0)
	position = Position{
		BuyPrice:     decimal.NewFromFloat(0.0),
		BuyDirection: BuyDirectionShort,
	}
	perf = position.PerformanceInPercentage(currentPrice, currentPrice)
	if perf != -100 {
		t.Fatalf("expected %v, got %v", -100, perf)
	}

}

func TestPerformanceAbsolute(t *testing.T) {
	t.Parallel()

	currentPrice := decimal.NewFromFloat(2.0)

	// Long
	position := Position{
		BuyPrice:     decimal.NewFromFloat(1.0),
		BuyDirection: BuyDirectionLong,
		Size:         1.00,
	}
	perf := position.PerformanceAbsolute(currentPrice, currentPrice)
	if perf != 1.0 {
		t.Fatalf("expected %v, got %v", 1.0, perf)
	}

	// Short
	position = Position{
		BuyPrice:     decimal.NewFromFloat(1.0),
		BuyDirection: BuyDirectionShort,
		Size:         1.00,
	}
	perf = position.PerformanceAbsolute(currentPrice, currentPrice)
	if perf != -1.0 {
		t.Fatalf("expected %v, got %v", -1.0, perf)
	}
}
