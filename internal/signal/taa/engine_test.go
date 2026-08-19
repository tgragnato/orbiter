//nolint:testpackage // test file accesses unexported symbols (abs, fakeStore, etc.)
package taa

import (
	"context"
	"errors"
	"testing"

	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/signal"
)

const symbolVWCEDE = "VWCE.DE"

// --- fakes ---

type fakeStore struct {
	holdings []portfolio.Holding
	err      error
}

func (f *fakeStore) ListHoldings(_ context.Context) ([]portfolio.Holding, error) {
	return f.holdings, f.err
}

func (f *fakeStore) ToggleAllocation(_ context.Context, _ int64) error   { return nil }
func (f *fakeStore) ToggleTAAEnabled(_ context.Context, _ string) error  { return nil }
func (f *fakeStore) TotalRealizedPnL(_ context.Context) (float64, error) { return 0, nil }

type fakePMC struct {
	prices map[string]float64
	err    error
}

func (f *fakePMC) PMC(_ context.Context, symbol string) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}

	val, ok := f.prices[symbol]
	if !ok {
		return 0, errors.New("not found")
	}

	return val, nil
}

type fakeConviction struct{ v float64 }

func (f *fakeConviction) Conviction(_ string) float64 { return f.v }

type fakeDispatcher struct {
	dispatched []signal.Message
}

func (f *fakeDispatcher) Dispatch(msg signal.Message) error {
	f.dispatched = append(f.dispatched, msg)

	return nil
}

type fakeSymbolProvider struct{ syms []string }

func (f *fakeSymbolProvider) Symbols() []string { return f.syms }

// --- tests ---

func TestEvaluateCorePMCFloorAlert(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 1, Symbol: symbolVWCEDE, Quantity: 1, MarketPrice: 80,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationCore, TAAEnabled: true,
			},
		},
		err: nil,
	}
	pmc := &fakePMC{prices: map[string]float64{symbolVWCEDE: 100}, err: nil} // price below PMC
	dispatch := &fakeDispatcher{dispatched: nil}

	engine := NewEngine(store, pmc, &fakeConviction{0}, nil, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, MaxBrokerFee: 0, Buffer: 0.01, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}

	if len(dispatch.dispatched) != 1 {
		t.Fatalf("expected 1 signal, got %d", len(dispatch.dispatched))
	}

	if dispatch.dispatched[0].Type != signal.TypeCorePMCFloorAlert {
		t.Errorf("signal type = %q, want CORE_PMC_FLOOR_ALERT", dispatch.dispatched[0].Type)
	}
}

func TestEvaluateCoreAboveFloor(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 1, Symbol: symbolVWCEDE, Quantity: 1, MarketPrice: 120,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationCore, TAAEnabled: true,
			},
		},
		err: nil,
	}
	pmc := &fakePMC{prices: map[string]float64{symbolVWCEDE: 100}, err: nil} // price above PMC
	dispatch := &fakeDispatcher{dispatched: nil}

	engine := NewEngine(store, pmc, &fakeConviction{0}, nil, dispatch, Config{
		TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}

	if len(dispatch.dispatched) != 0 {
		t.Errorf("expected no signals for core above floor, got %d", len(dispatch.dispatched))
	}
}

func TestEvaluateSatelliteGatePasses(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 2, Symbol: "ZPRV.DE", Quantity: 1, MarketPrice: 50,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
			},
		},
		err: nil,
	}
	// conviction=0.9 >> friction=0.26*0.001+0.001+0.01 ≈ 0.011
	dispatch := &fakeDispatcher{dispatched: nil}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.9}, nil, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, MaxBrokerFee: 0, Buffer: 0.01, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if len(dispatch.dispatched) != 1 {
		t.Fatalf("expected 1 rebalance signal, got %d", len(dispatch.dispatched))
	}

	if dispatch.dispatched[0].Type != signal.TypeRebalance {
		t.Errorf("signal type = %q, want REBALANCE", dispatch.dispatched[0].Type)
	}
}

func TestEvaluateSatelliteGateBlocked(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 2, Symbol: "ZPRV.DE", Quantity: 1, MarketPrice: 50,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
			},
		},
		err: nil,
	}
	// conviction=0.005 << friction ≈ 0.011 → should be blocked
	dispatch := &fakeDispatcher{dispatched: nil}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.005}, nil, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, MaxBrokerFee: 0, Buffer: 0.01, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if len(dispatch.dispatched) != 0 {
		t.Errorf("expected no signals when blocked by friction, got %d", len(dispatch.dispatched))
	}
}

func TestEvaluateStoreError(t *testing.T) {
	t.Parallel()

	store := &fakeStore{holdings: nil, err: errors.New("db error")}
	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0}, nil, &fakeDispatcher{dispatched: nil}, Config{
		TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err == nil {
		t.Error("expected error from store, got nil")
	}
}

func TestEvaluateCoreNoPMCSkipsFloor(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 1, Symbol: "X", Quantity: 1, MarketPrice: 50,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationCore, TAAEnabled: true,
			},
		},
		err: nil,
	}
	dispatch := &fakeDispatcher{dispatched: nil}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0}, nil, dispatch, Config{
		TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if len(dispatch.dispatched) != 0 {
		t.Errorf("expected no signals when PMC unavailable, got %d", len(dispatch.dispatched))
	}
}

func TestNullPMCReader(t *testing.T) {
	t.Parallel()

	r := NullPMCReader{}

	_, err := r.PMC(context.Background(), "X")
	if err == nil {
		t.Error("NullPMCReader.PMC should always return error")
	}
}

func TestAbsHelper(t *testing.T) {
	t.Parallel()

	if abs(-0.5) != 0.5 {
		t.Error("abs(-0.5) should be 0.5")
	}

	if abs(0.7) != 0.7 {
		t.Error("abs(0.7) should be 0.7")
	}
}

//nolint:dupl // structurally similar test for a different filter condition
func TestEvaluateSatellitePerOrderFeeCapNotAggregate(t *testing.T) {
	t.Parallel()
	// A: 100 units @ 200 EUR = 20 000 EUR → capped rate = 18.90/20 000 = 0.000945
	// → friction 0.000945 < conviction 0.005 → signal
	// B: 100 units @   1 EUR = 100 EUR → 18.90/100 = 0.189 > BrokerFeePercent 0.01
	// → uncapped rate 0.01 → friction 0.01 > 0.005 → blocked
	//
	// If aggregate satellite NAV were used instead:
	//   capped rate = 18.90/20 100 ≈ 0.00094 → friction ≈ 0.00094 for BOTH (wrong).
	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 1, Symbol: "A", Quantity: 100, MarketPrice: 200,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
			},
			{
				ID: 2, Symbol: "B", Quantity: 100, MarketPrice: 1,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
			},
		},
		err: nil,
	}
	dispatch := &fakeDispatcher{dispatched: nil}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.005}, nil, dispatch, Config{
		TaxRate: 0, BrokerFeePercent: 0.01, MaxBrokerFee: 18.90, Buffer: 0, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if len(dispatch.dispatched) != 1 {
		t.Errorf("expected 1 signal (only A passes per-order gate), got %d — aggregate NAV cap would produce 2",
			len(dispatch.dispatched))
	}

	if len(dispatch.dispatched) == 1 && dispatch.dispatched[0].Instrument != "A" {
		t.Errorf("expected signal for A, got %q", dispatch.dispatched[0].Instrument)
	}
}

//nolint:dupl // structurally similar test for a different filter condition
func TestEvaluateSkipsTAADisabledHoldings(t *testing.T) {
	t.Parallel()

	// A has TAAEnabled=false → skipped; B has TAAEnabled=true → evaluated and signal emitted.
	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 1, Symbol: "A", Quantity: 10, MarketPrice: 100,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: false,
			},
			{
				ID: 2, Symbol: "B", Quantity: 10, MarketPrice: 100,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
			},
		},
		err: nil,
	}
	dispatch := &fakeDispatcher{dispatched: nil}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.9}, nil, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, MaxBrokerFee: 0, Buffer: 0.01, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if len(dispatch.dispatched) != 1 {
		t.Errorf("expected 1 signal (only B with TAAEnabled=true), got %d", len(dispatch.dispatched))
	}

	if len(dispatch.dispatched) == 1 && dispatch.dispatched[0].Instrument != "B" {
		t.Errorf("expected signal for B, got %q", dispatch.dispatched[0].Instrument)
	}
}

//nolint:dupl // structurally similar test for a different filter condition
func TestEvaluateSkipsZeroQtyHoldings(t *testing.T) {
	t.Parallel()

	// CLOSED has qty=0 → skipped; OPEN has qty=5 → evaluated and signal emitted.
	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 1, Symbol: "CLOSED", Quantity: 0, MarketPrice: 100,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
			},
			{
				ID: 2, Symbol: "OPEN", Quantity: 5, MarketPrice: 100,
				PMC: 0, Currency: "", AllocationType: portfolio.AllocationSatellite, TAAEnabled: true,
			},
		},
		err: nil,
	}
	dispatch := &fakeDispatcher{dispatched: nil}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.9}, nil, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, MaxBrokerFee: 0, Buffer: 0.01, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if len(dispatch.dispatched) != 1 {
		t.Errorf("expected 1 signal (only OPEN passes), got %d", len(dispatch.dispatched))
	}

	if len(dispatch.dispatched) == 1 && dispatch.dispatched[0].Instrument != "OPEN" {
		t.Errorf("expected signal for OPEN, got %q", dispatch.dispatched[0].Instrument)
	}
}

func TestEvaluateCoreUsesHoldingPMCWhenPresent(t *testing.T) {
	t.Parallel()

	// PMC is embedded in the holding (100) and market price (80) is below it.
	store := &fakeStore{
		holdings: []portfolio.Holding{
			{
				ID: 1, Symbol: symbolVWCEDE, Quantity: 1, MarketPrice: 80,
				PMC: 100, Currency: "", AllocationType: portfolio.AllocationCore, TAAEnabled: true,
			},
		},
		err: nil,
	}
	dispatch := &fakeDispatcher{dispatched: nil}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0}, nil, dispatch, Config{
		TaxRate: 0, BrokerFeePercent: 0, MaxBrokerFee: 0, Buffer: 0, Currency: "",
	})

	err := engine.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}

	if len(dispatch.dispatched) != 1 {
		t.Fatalf("expected 1 PMC floor alert, got %d", len(dispatch.dispatched))
	}

	if dispatch.dispatched[0].Type != signal.TypeCorePMCFloorAlert {
		t.Errorf("signal type = %q, want CORE_PMC_FLOOR_ALERT", dispatch.dispatched[0].Type)
	}
}
