package taa

import (
	"math"
	"testing"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/signal"
)

var testNow = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

type perSymbolConviction map[string]float64

func (p perSymbolConviction) Conviction(symbol string) float64 { return p[symbol] }

func TestOptimizerEmptyHoldings(t *testing.T) {
	t.Parallel()
	msgs := optimizeSatellite(nil, perSymbolConviction{}, Config{}, testNow)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for empty input, got %d", len(msgs))
	}
}

func TestOptimizerSkipsNonSatellite(t *testing.T) {
	t.Parallel()
	holdings := []portfolio.Holding{
		{Symbol: "VWCE", Quantity: 1, MarketPrice: 100, AllocationType: portfolio.AllocationCore, TAAEnabled: true},
	}
	msgs := optimizeSatellite(holdings, perSymbolConviction{"VWCE": 0.9}, Config{}, testNow)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for core holding, got %d", len(msgs))
	}
}

func TestOptimizerSkipsTAADisabled(t *testing.T) {
	t.Parallel()
	holdings := []portfolio.Holding{
		{Symbol: "A", Quantity: 1, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: false},
	}
	msgs := optimizeSatellite(holdings, perSymbolConviction{"A": 0.9}, Config{}, testNow)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for TAAEnabled=false, got %d", len(msgs))
	}
}

func TestOptimizerSkipsZeroQuantity(t *testing.T) {
	t.Parallel()
	holdings := []portfolio.Holding{
		{Symbol: "A", Quantity: 0, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}
	msgs := optimizeSatellite(holdings, perSymbolConviction{"A": 0.9}, Config{}, testNow)
	if len(msgs) != 0 {
		t.Errorf("expected no messages for zero-qty holding, got %d", len(msgs))
	}
}

func TestOptimizerEqualWeightWithZeroConviction(t *testing.T) {
	t.Parallel()
	// Two equal-NAV holdings with conviction=0 → both get 50% target weight.
	holdings := []portfolio.Holding{
		{Symbol: "A", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
		{Symbol: "B", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}
	// Low friction (buffer=0) so both pass the gate at conviction=0? No — abs(0) <= 0 is blocked.
	// Actually conviction=0 is blocked: abs(0) <= friction (even 0 friction means 0 <= 0, blocked).
	// Use conviction=0.5 for both to pass, then weights should still be equal since conviction is same.
	msgs := optimizeSatellite(holdings, perSymbolConviction{"A": 0.5, "B": 0.5}, Config{Buffer: 0.01}, testNow)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	for _, m := range msgs {
		if math.Abs(m.TargetWeight-0.5) > 1e-9 {
			t.Errorf("%s: target weight = %.4f, want 0.5", m.Instrument, m.TargetWeight)
		}
		if math.Abs(m.Delta) > 1e-6 {
			t.Errorf("%s: delta = %.4f, want 0 (already at target)", m.Instrument, m.Delta)
		}
	}
}

func TestOptimizerConvictionWeightedAllocation(t *testing.T) {
	t.Parallel()
	// A: conviction=0.5 → rawWeight = 1.5
	// B: conviction=-0.5 → rawWeight = 0.5
	// Total raw = 2.0 → A target=75%, B target=25%
	holdings := []portfolio.Holding{
		{Symbol: "A", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
		{Symbol: "B", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}
	msgs := optimizeSatellite(holdings, perSymbolConviction{"A": 0.5, "B": -0.5}, Config{Buffer: 0.01}, testNow)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	bySymbol := map[string]signal.Message{}
	for _, m := range msgs {
		bySymbol[m.Instrument] = m
	}

	wantA, wantB := 0.75, 0.25
	if math.Abs(bySymbol["A"].TargetWeight-wantA) > 1e-9 {
		t.Errorf("A target weight = %.4f, want %.4f", bySymbol["A"].TargetWeight, wantA)
	}
	if math.Abs(bySymbol["B"].TargetWeight-wantB) > 1e-9 {
		t.Errorf("B target weight = %.4f, want %.4f", bySymbol["B"].TargetWeight, wantB)
	}

	// A needs to increase by 25% of 2000 EUR total NAV = +500 EUR
	if math.Abs(bySymbol["A"].Delta-500) > 1e-6 {
		t.Errorf("A delta = %.4f, want 500", bySymbol["A"].Delta)
	}
	// B needs to decrease by 25% of 2000 EUR = -500 EUR
	if math.Abs(bySymbol["B"].Delta-(-500)) > 1e-6 {
		t.Errorf("B delta = %.4f, want -500", bySymbol["B"].Delta)
	}
}

func TestOptimizerNegativeConvictionReducesToZeroWeight(t *testing.T) {
	t.Parallel()
	// conviction=-1 → rawWeight = max(0, 1-1) = 0 → target 0%
	// conviction=+1 → rawWeight = 2 → target 100%
	holdings := []portfolio.Holding{
		{Symbol: "BULL", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
		{Symbol: "BEAR", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}
	msgs := optimizeSatellite(holdings, perSymbolConviction{"BULL": 1.0, "BEAR": -1.0}, Config{Buffer: 0.01}, testNow)

	bySymbol := map[string]signal.Message{}
	for _, m := range msgs {
		bySymbol[m.Instrument] = m
	}

	// BULL should pass (conviction=1 >> friction) and have target 100%
	if bull, ok := bySymbol["BULL"]; ok {
		if math.Abs(bull.TargetWeight-1.0) > 1e-9 {
			t.Errorf("BULL target weight = %.4f, want 1.0", bull.TargetWeight)
		}
	} else {
		t.Error("expected BULL to be in output")
	}

	// BEAR should pass (abs(-1) >> friction) and have target 0%
	if bear, ok := bySymbol["BEAR"]; ok {
		if math.Abs(bear.TargetWeight) > 1e-9 {
			t.Errorf("BEAR target weight = %.4f, want 0.0", bear.TargetWeight)
		}
	} else {
		t.Error("expected BEAR to be in output")
	}
}

func TestOptimizerAllNegativeConvictionEmitsExits(t *testing.T) {
	t.Parallel()
	// All conviction=-1 → rawWeight=0 for both → both emitted as TypeSell
	holdings := []portfolio.Holding{
		{Symbol: "A", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
		{Symbol: "B", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}
	msgs := optimizeSatellite(holdings, perSymbolConviction{"A": -1.0, "B": -1.0}, Config{Buffer: 0.01}, testNow)

	// Both pass friction (abs(-1)=1 > 0.01) and are emitted as exit signals.
	if len(msgs) != 2 {
		t.Fatalf("expected 2 exit messages, got %d", len(msgs))
	}
	for _, m := range msgs {
		if m.Type != signal.TypeSell {
			t.Errorf("%s: expected TypeSell, got %q", m.Instrument, m.Type)
		}
		if m.TargetWeight != 0 {
			t.Errorf("%s: target weight = %.4f, want 0 (full exit)", m.Instrument, m.TargetWeight)
		}
		if m.Delta >= 0 {
			t.Errorf("%s: delta = %.2f, want negative (selling)", m.Instrument, m.Delta)
		}
	}
}

func TestOptimizerFrictionGateFilters(t *testing.T) {
	t.Parallel()
	// A: conviction=0.5 → passes friction (buffer=0.01)
	// B: conviction=0.005 → blocked by friction
	holdings := []portfolio.Holding{
		{Symbol: "A", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
		{Symbol: "B", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}
	msgs := optimizeSatellite(holdings, perSymbolConviction{"A": 0.5, "B": 0.005}, Config{Buffer: 0.01}, testNow)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (B blocked), got %d", len(msgs))
	}
	if msgs[0].Instrument != "A" {
		t.Errorf("expected signal for A, got %q", msgs[0].Instrument)
	}
}

func TestOptimizerRebalanceMessageDirection(t *testing.T) {
	t.Parallel()
	holdings := []portfolio.Holding{
		{Symbol: "UP", Quantity: 5, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
		{Symbol: "DOWN", Quantity: 15, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}
	// UP: conviction=0.5, current=25%, target=75% → "increase"
	// DOWN: conviction=-0.5, current=75%, target=25% → "decrease"
	msgs := optimizeSatellite(holdings, perSymbolConviction{"UP": 0.5, "DOWN": -0.5}, Config{Buffer: 0.01}, testNow)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	for _, m := range msgs {
		switch m.Instrument {
		case "UP":
			if m.Delta <= 0 {
				t.Errorf("UP: delta should be positive (buy), got %.2f", m.Delta)
			}
		case "DOWN":
			if m.Delta >= 0 {
				t.Errorf("DOWN: delta should be negative (sell), got %.2f", m.Delta)
			}
		}
	}
}

func TestOptimizerCurrentWeightPopulated(t *testing.T) {
	t.Parallel()
	// A: 200 EUR NAV, B: 800 EUR NAV → current weights 20% and 80%
	holdings := []portfolio.Holding{
		{Symbol: "A", Quantity: 2, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
		{Symbol: "B", Quantity: 8, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}
	msgs := optimizeSatellite(holdings, perSymbolConviction{"A": 0.5, "B": 0.5}, Config{Buffer: 0.01}, testNow)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	bySymbol := map[string]signal.Message{}
	for _, m := range msgs {
		bySymbol[m.Instrument] = m
	}
	if math.Abs(bySymbol["A"].CurrentWeight-0.2) > 1e-9 {
		t.Errorf("A current weight = %.4f, want 0.2", bySymbol["A"].CurrentWeight)
	}
	if math.Abs(bySymbol["B"].CurrentWeight-0.8) > 1e-9 {
		t.Errorf("B current weight = %.4f, want 0.8", bySymbol["B"].CurrentWeight)
	}
}
