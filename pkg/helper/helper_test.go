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

func TestGetPercentile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		n          []float64
		percentile int
		want       float64
	}{
		{"Empty slice", []float64{}, 50, 0.0},
		{"Single element", []float64{10.0}, 50, 10.0},
		{"Min percentile (0)", []float64{10.0, 20.0, 30.0}, 0, 10.0},
		{"Max percentile (100)", []float64{10.0, 20.0, 30.0}, 100, 30.0},
		{"Median (50) odd count", []float64{10.0, 20.0, 30.0}, 50, 20.0},
		{"Median (50) even count", []float64{10.0, 20.0, 30.0, 40.0}, 50, 20.0},
		{"25th percentile", []float64{10.0, 20.0, 30.0, 40.0}, 25, 10.0},
		{"75th percentile", []float64{10.0, 20.0, 30.0, 40.0}, 75, 30.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPercentile(tt.n, tt.percentile)
			if got != tt.want {
				t.Errorf("GetPercentile() = %v, want %v", got, tt.want)
			}
		})
	}
}
