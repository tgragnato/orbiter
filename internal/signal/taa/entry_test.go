//nolint:testpackage // test file accesses unexported symbols (evaluateEntries, perSymbolConviction, testNow)
package taa

import (
	"context"
	"math"
	"testing"

	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/signal"
)

func TestEvaluateEntriesNoTrackedSymbols(t *testing.T) {
	t.Parallel()

	msgs := evaluateEntries(nil, nil, perSymbolConviction{}, Config{
		TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0, Currency: "",
	}, testNow)

	if len(msgs) != 0 {
		t.Errorf("expected no messages for empty symbol list, got %d", len(msgs))
	}
}

func TestEvaluateEntriesSkipsTAADisabledSymbols(t *testing.T) {
	t.Parallel()

	// A has TAAEnabled=false and Quantity=0 (closed position) — user opted out, no entry signal.
	holdings := []portfolio.Holding{
		{
			ID: 0, Symbol: "A", Quantity: 0, MarketPrice: 100,
			PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: false,
		},
	}

	msgs := evaluateEntries(holdings, []string{"A"}, perSymbolConviction{"A": 0.9}, Config{
		TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0.01, Currency: "",
	}, testNow)

	if len(msgs) != 0 {
		t.Errorf("expected no entry signal for TAAEnabled=false symbol, got %d", len(msgs))
	}
}

func TestEvaluateEntriesSkipsHeldSymbols(t *testing.T) {
	t.Parallel()

	holdings := []portfolio.Holding{
		{
			ID: 0, Symbol: "A", Quantity: 10, MarketPrice: 100,
			PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
		},
	}

	msgs := evaluateEntries(holdings, []string{"A"}, perSymbolConviction{"A": 0.9}, Config{
		TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0.01, Currency: "",
	}, testNow)

	if len(msgs) != 0 {
		t.Errorf("expected no entry signal for already-held symbol, got %d", len(msgs))
	}
}

func TestEvaluateEntriesEmitsForUnownedHighConviction(t *testing.T) {
	t.Parallel()

	holdings := []portfolio.Holding{
		{
			ID: 0, Symbol: "A", Quantity: 10, MarketPrice: 100,
			PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
		},
	}

	msgs := evaluateEntries(
		holdings,
		[]string{"A", "B"},
		perSymbolConviction{"A": 0.3, "B": 0.8},
		Config{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0.01, Currency: ""},
		testNow,
	)

	if len(msgs) != 1 {
		t.Fatalf("expected 1 entry signal for B, got %d", len(msgs))
	}

	if msgs[0].Instrument != "B" {
		t.Errorf("expected entry signal for B, got %q", msgs[0].Instrument)
	}

	if msgs[0].Type != signal.TypeBuy {
		t.Errorf("expected TypeEntry, got %q", msgs[0].Type)
	}
}

func TestEvaluateEntriesFrictionBlocksLowConviction(t *testing.T) {
	t.Parallel()

	msgs := evaluateEntries(
		nil,
		[]string{"B"},
		perSymbolConviction{"B": 0.005},
		Config{BrokerFeePercent: 0.001, TaxRate: 0.26, MaxBrokerFee: 0, Buffer: 0.01, Currency: ""},
		testNow,
	)

	if len(msgs) != 0 {
		t.Errorf("expected no entry signal when conviction below friction, got %d", len(msgs))
	}
}

func TestEvaluateEntriesDeltaEURScalesWithSatelliteNAV(t *testing.T) {
	t.Parallel()

	// Satellite NAV = 1000 EUR (10 units @ 100 EUR)
	// A (held) conviction=0 → rawWeight=1
	// B (unowned) conviction=1 → rawWeight=2
	// total raw = 3 → B target = 2/3, delta = 2/3 * 1000
	holdings := []portfolio.Holding{
		{
			ID: 0, Symbol: "A", Quantity: 10, MarketPrice: 100,
			PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
		},
	}

	msgs := evaluateEntries(
		holdings,
		[]string{"A", "B"},
		perSymbolConviction{"A": 0.0, "B": 1.0},
		Config{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0.01, Currency: ""},
		testNow,
	)

	if len(msgs) != 1 {
		t.Fatalf("expected 1 entry signal, got %d", len(msgs))
	}

	wantTarget := 2.0 / 3.0
	wantDelta := wantTarget * 1000.0

	if math.Abs(msgs[0].TargetWeight-wantTarget) > 1e-9 {
		t.Errorf("target weight = %.6f, want %.6f", msgs[0].TargetWeight, wantTarget)
	}

	if math.Abs(msgs[0].Delta-wantDelta) > 1e-4 {
		t.Errorf("delta = %.4f, want %.4f", msgs[0].Delta, wantDelta)
	}
}

func TestEvaluateEntriesNoHoldingsDeltaEURIsZero(t *testing.T) {
	t.Parallel()

	// No existing satellite holdings → totalSatelliteNAV = 0 → delta = 0
	msgs := evaluateEntries(
		nil,
		[]string{"B"},
		perSymbolConviction{"B": 0.9},
		Config{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0.01, Currency: ""},
		testNow,
	)

	if len(msgs) != 1 {
		t.Fatalf("expected 1 entry signal, got %d", len(msgs))
	}

	if msgs[0].Delta != 0 {
		t.Errorf("delta = %.4f, want 0 when no satellite holdings exist", msgs[0].Delta)
	}
}

func TestEvaluateEntriesNegativeConvictionEmitsEntry(t *testing.T) {
	t.Parallel()

	msgs := evaluateEntries(
		nil,
		[]string{"B"},
		perSymbolConviction{"B": -0.8},
		Config{TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0.01, Currency: ""},
		testNow,
	)

	if len(msgs) != 1 {
		t.Fatalf("expected 1 entry signal for short direction, got %d", len(msgs))
	}

	if msgs[0].Type != signal.TypeBuy {
		t.Errorf("expected TypeEntry, got %q", msgs[0].Type)
	}
}

func TestEvaluateEntriesEngineIntegration(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 1, Symbol: "A", Quantity: 10, MarketPrice: 100,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
			},
		},
		err: nil,
	}
	dispatch := &fakeDispatcher{dispatched: nil}
	conv := perSymbolConviction{"A": 0.3, "B": 0.9}

	engine := NewEngine(
		store,
		&NullPMCReader{},
		conv,
		&fakeSymbolProvider{syms: []string{"A", "B"}},
		dispatch,
		Config{TaxRate: 0.26, BrokerFeePercent: 0.001, MaxBrokerFee: 0, Buffer: 0.01, Currency: ""},
	)

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	var entries []signal.Message

	for _, msg := range dispatch.dispatched {
		if msg.Type == signal.TypeBuy {
			entries = append(entries, msg)
		}
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry signal for B, got %d", len(entries))
	}

	if len(entries) == 1 && entries[0].Instrument != "B" {
		t.Errorf("expected entry for B, got %q", entries[0].Instrument)
	}
}
