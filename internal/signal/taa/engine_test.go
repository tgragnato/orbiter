package taa

import (
	"context"
	"errors"
	"testing"

	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/signal"
)

// --- fakes ---

type fakeStore struct {
	holdings []portfolio.Holding
	err      error
}

func (f *fakeStore) ListHoldings(_ context.Context) ([]portfolio.Holding, error) {
	return f.holdings, f.err
}

func (f *fakeStore) ToggleAllocation(_ context.Context, _ int64) error      { return nil }
func (f *fakeStore) ToggleTAAEnabled(_ context.Context, _ string) error     { return nil }
func (f *fakeStore) TotalRealizedPnL(_ context.Context) (float64, error)    { return 0, nil }

type fakePMC struct {
	prices map[string]float64
	err    error
}

func (f *fakePMC) PMC(_ context.Context, symbol string) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	v, ok := f.prices[symbol]
	if !ok {
		return 0, errors.New("not found")
	}
	return v, nil
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

// --- tests ---

func TestEvaluateCorePMCFloorAlert(t *testing.T) {
	t.Parallel()
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "VWCE.DE", Quantity: 1, MarketPrice: 80, AllocationType: portfolio.AllocationCore, TAAEnabled: true},
	}}
	pmc := &fakePMC{prices: map[string]float64{"VWCE.DE": 100}} // price below PMC
	dispatch := &fakeDispatcher{}

	engine := NewEngine(store, pmc, &fakeConviction{0}, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, Buffer: 0.01, RebalanceThreshold: 0.05,
	}, nil)
	if err := engine.Evaluate(context.Background()); err != nil {
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
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "VWCE.DE", Quantity: 1, MarketPrice: 120, AllocationType: portfolio.AllocationCore, TAAEnabled: true},
	}}
	pmc := &fakePMC{prices: map[string]float64{"VWCE.DE": 100}} // price above PMC
	dispatch := &fakeDispatcher{}

	engine := NewEngine(store, pmc, &fakeConviction{0}, dispatch, Config{}, nil)
	if err := engine.Evaluate(context.Background()); err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if len(dispatch.dispatched) != 0 {
		t.Errorf("expected no signals for core above floor, got %d", len(dispatch.dispatched))
	}
}

func TestEvaluateSatelliteGatePasses(t *testing.T) {
	t.Parallel()
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 2, Symbol: "ZPRV.DE", Quantity: 1, MarketPrice: 50, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}}
	// conviction=0.9 >> friction=0.26*0.001+0.001+0.01 ≈ 0.011
	dispatch := &fakeDispatcher{}
	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.9}, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, Buffer: 0.01,
	}, nil)
	if err := engine.Evaluate(context.Background()); err != nil {
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
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 2, Symbol: "ZPRV.DE", Quantity: 1, MarketPrice: 50, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}}
	// conviction=0.005 << friction ≈ 0.011 → should be blocked
	dispatch := &fakeDispatcher{}
	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.005}, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, Buffer: 0.01,
	}, nil)
	if err := engine.Evaluate(context.Background()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(dispatch.dispatched) != 0 {
		t.Errorf("expected no signals when blocked by friction, got %d", len(dispatch.dispatched))
	}
}

func TestEvaluateStoreError(t *testing.T) {
	t.Parallel()
	store := &fakeStore{err: errors.New("db error")}
	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0}, &fakeDispatcher{}, Config{}, nil)
	if err := engine.Evaluate(context.Background()); err == nil {
		t.Error("expected error from store, got nil")
	}
}

func TestEvaluateCoreNoPMCSkipsFloor(t *testing.T) {
	t.Parallel()
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "X", Quantity: 1, MarketPrice: 50, AllocationType: portfolio.AllocationCore, TAAEnabled: true},
	}}
	dispatch := &fakeDispatcher{}
	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0}, dispatch, Config{}, nil)
	if err := engine.Evaluate(context.Background()); err != nil {
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

func TestConstantConviction(t *testing.T) {
	t.Parallel()
	c := ConstantConviction{V: 0.5}
	if c.Conviction("anything") != 0.5 {
		t.Error("ConstantConviction should return constant value")
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

// --- TargetReader / drift gate tests ---

type fakeTargetReader struct {
	targets            CoreSatelliteTargets
	rebalanceThreshold float64 // 0 means "not set"; engine falls back to Config value
	err                error
}

func (f *fakeTargetReader) GetCoreSatelliteTargets(_ context.Context) (CoreSatelliteTargets, error) {
	return f.targets, f.err
}

func (f *fakeTargetReader) GetRebalanceThreshold(_ context.Context) (float64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.rebalanceThreshold, nil
}

func TestEvaluateDriftGateBlocksWhenWithinThreshold(t *testing.T) {
	t.Parallel()
	// Core holding worth 80, satellite worth 20 → 80% core exactly on target.
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "CORE1", Quantity: 1, MarketPrice: 80, AllocationType: portfolio.AllocationCore, TAAEnabled: true},
		{ID: 2, Symbol: "SAT1", Quantity: 1, MarketPrice: 20, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}}
	dispatch := &fakeDispatcher{}
	targets := &fakeTargetReader{targets: CoreSatelliteTargets{CoreRatio: 0.8, SatelliteRatio: 0.2}}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.9}, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, Buffer: 0.01, RebalanceThreshold: 0.05,
	}, targets)

	if err := engine.Evaluate(context.Background()); err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	// Drift = |0.80 - 0.80| = 0.0 < 0.05 → no signals emitted.
	if len(dispatch.dispatched) != 0 {
		t.Errorf("expected 0 signals when drift within threshold, got %d", len(dispatch.dispatched))
	}
}

func TestEvaluateDriftGatePassesWhenAboveThreshold(t *testing.T) {
	t.Parallel()
	// Core 60%, target 80% → drift = 0.20 > 0.05 → evaluate per-holding.
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "SAT1", Quantity: 1, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}}
	dispatch := &fakeDispatcher{}
	targets := &fakeTargetReader{targets: CoreSatelliteTargets{CoreRatio: 0.8, SatelliteRatio: 0.2}}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.9}, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, Buffer: 0.01, RebalanceThreshold: 0.05,
	}, targets)

	if err := engine.Evaluate(context.Background()); err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	// Drift = |0.0 - 0.8| = 0.8 > 0.05 → satellite evaluated → rebalance emitted.
	if len(dispatch.dispatched) != 1 {
		t.Errorf("expected 1 signal above drift threshold, got %d", len(dispatch.dispatched))
	}
}

func TestEvaluateDynamicThresholdOverridesConfig(t *testing.T) {
	t.Parallel()
	// Portfolio is exactly on target (drift = 0), but the DB threshold is 0.
	// The Config threshold is 0.05, so without the dynamic override the gate
	// would block (drift 0 < 0.05). With the DB threshold set to 0 the
	// condition dynThreshold > 0 is false → falls back to Config → still blocks.
	// Set DB threshold to a value that forces the gate OPEN: threshold = 0 is
	// not useful; instead, set it very small so drift > threshold.
	// Core 60%, target 80% → drift 0.20; static Config threshold 0.99 blocks.
	// Dynamic threshold 0.10 passes → satellite signal emitted.
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "SAT1", Quantity: 1, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}}
	dispatch := &fakeDispatcher{}
	targets := &fakeTargetReader{
		targets:            CoreSatelliteTargets{CoreRatio: 0.8, SatelliteRatio: 0.2},
		rebalanceThreshold: 0.10, // DB value overrides Config value of 0.99
	}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.9}, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, Buffer: 0.01,
		RebalanceThreshold: 0.99, // would block without the DB override
	}, targets)

	if err := engine.Evaluate(context.Background()); err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	// drift = |0.0/100 - 0.8| = 0.8 > DB threshold 0.10 → gate passes → 1 signal.
	if len(dispatch.dispatched) != 1 {
		t.Errorf("expected 1 signal with dynamic threshold override, got %d", len(dispatch.dispatched))
	}
}

func TestEvaluateSatellitePerOrderFeeCapNotAggregate(t *testing.T) {
	t.Parallel()
	// A: 100 units @ 200 EUR = 20 000 EUR → capped rate = 18.90/20 000 = 0.000945 → friction 0.000945 < conviction 0.005 → signal
	// B: 100 units @   1 EUR =    100 EUR → 18.90/100 = 0.189 > BrokerFeePercent 0.01 → uncapped rate 0.01 → friction 0.01 > 0.005 → blocked
	//
	// If aggregate satellite NAV (20 100 EUR) were used instead:
	//   capped rate = 18.90/20 100 ≈ 0.00094 → friction ≈ 0.00094 for BOTH → two signals (wrong).
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "A", Quantity: 100, MarketPrice: 200, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
		{ID: 2, Symbol: "B", Quantity: 100, MarketPrice: 1, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}}
	dispatch := &fakeDispatcher{}
	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.005}, dispatch, Config{
		TaxRate: 0, BrokerFeePercent: 0.01, MaxBrokerFeeEUR: 18.90, Buffer: 0,
	}, nil)
	if err := engine.Evaluate(context.Background()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(dispatch.dispatched) != 1 {
		t.Errorf("expected 1 signal (only A passes per-order gate), got %d — aggregate NAV cap would produce 2", len(dispatch.dispatched))
	}
	if len(dispatch.dispatched) == 1 && dispatch.dispatched[0].Instrument != "A" {
		t.Errorf("expected signal for A, got %q", dispatch.dispatched[0].Instrument)
	}
}

func TestEvaluateSkipsTAADisabledHoldings(t *testing.T) {
	t.Parallel()
	// A has TAAEnabled=false → skipped; B has TAAEnabled=true → evaluated and signal emitted.
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "A", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: false},
		{ID: 2, Symbol: "B", Quantity: 10, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}}
	dispatch := &fakeDispatcher{}
	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.9}, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, Buffer: 0.01,
	}, nil)
	if err := engine.Evaluate(context.Background()); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(dispatch.dispatched) != 1 {
		t.Errorf("expected 1 signal (only B with TAAEnabled=true), got %d", len(dispatch.dispatched))
	}
	if len(dispatch.dispatched) == 1 && dispatch.dispatched[0].Instrument != "B" {
		t.Errorf("expected signal for B, got %q", dispatch.dispatched[0].Instrument)
	}
}

func TestEvaluateSkipsZeroQtyHoldings(t *testing.T) {
	t.Parallel()
	// CLOSED has qty=0 → skipped; OPEN has qty=5 → evaluated and signal emitted.
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "CLOSED", Quantity: 0, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
		{ID: 2, Symbol: "OPEN", Quantity: 5, MarketPrice: 100, AllocationType: portfolio.AllocationSatellite, TAAEnabled: true},
	}}
	dispatch := &fakeDispatcher{}
	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0.9}, dispatch, Config{
		TaxRate: 0.26, BrokerFeePercent: 0.001, Buffer: 0.01,
	}, nil)
	if err := engine.Evaluate(context.Background()); err != nil {
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
	store := &fakeStore{holdings: []portfolio.Holding{
		{ID: 1, Symbol: "VWCE.DE", Quantity: 1, MarketPrice: 80, PMC: 100, AllocationType: portfolio.AllocationCore, TAAEnabled: true},
	}}
	dispatch := &fakeDispatcher{}

	engine := NewEngine(store, &NullPMCReader{}, &fakeConviction{0}, dispatch, Config{
		RebalanceThreshold: 0.05,
	}, nil)

	if err := engine.Evaluate(context.Background()); err != nil {
		t.Fatalf("Evaluate error: %v", err)
	}
	if len(dispatch.dispatched) != 1 {
		t.Fatalf("expected 1 PMC floor alert, got %d", len(dispatch.dispatched))
	}
	if dispatch.dispatched[0].Type != signal.TypeCorePMCFloorAlert {
		t.Errorf("signal type = %q, want CORE_PMC_FLOOR_ALERT", dispatch.dispatched[0].Type)
	}
}
