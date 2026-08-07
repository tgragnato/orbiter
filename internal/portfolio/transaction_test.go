package portfolio

import (
	"math"
	"testing"
	"time"
)

var txBase = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func TestComputeHoldingStatesSingleBuy(t *testing.T) {
	t.Parallel()
	txs := []Transaction{
		{Symbol: "VWCE.MI", Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 2,
			AllocationType: AllocationCore, ExecutedAt: txBase},
	}
	states := ComputeHoldingStates(txs, nil)
	s := states["VWCE.MI"]
	if s.Quantity != 10 {
		t.Fatalf("qty = %f, want 10", s.Quantity)
	}
	// PMC = (0 + 10*100 + 2) / 10 = 100.2
	if s.PMC != 100.2 {
		t.Fatalf("pmc = %f, want 100.2", s.PMC)
	}
	if s.AllocationType != AllocationCore {
		t.Fatalf("alloc = %q, want CORE", s.AllocationType)
	}
}

func TestComputeHoldingStatesAccumulateBuys(t *testing.T) {
	t.Parallel()
	txs := []Transaction{
		{Symbol: "X", Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase},
		{Symbol: "X", Type: TransactionBuy, Quantity: 5, Price: 110, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour)},
	}
	states := ComputeHoldingStates(txs, nil)
	s := states["X"]
	if s.Quantity != 15 {
		t.Fatalf("qty = %f, want 15", s.Quantity)
	}
	// PMC = (10*100 + 5*110) / 15 = 1550/15
	want := (10.0*100 + 5*110) / 15
	if math.Abs(s.PMC-want) > 1e-9 {
		t.Fatalf("pmc = %f, want %f", s.PMC, want)
	}
}

func TestComputeHoldingStatesPartialSell(t *testing.T) {
	t.Parallel()
	txs := []Transaction{
		{Symbol: "X", Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase},
		{Symbol: "X", Type: TransactionSell, Quantity: 4, Price: 120, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour)},
	}
	states := ComputeHoldingStates(txs, nil)
	s := states["X"]
	if s.Quantity != 6 {
		t.Fatalf("qty = %f, want 6", s.Quantity)
	}
	// PMC unchanged on sell
	if s.PMC != 100 {
		t.Fatalf("pmc = %f, want 100 (PMC unchanged on sell)", s.PMC)
	}
}

func TestComputeHoldingStatesFullSellResetsState(t *testing.T) {
	t.Parallel()
	txs := []Transaction{
		{Symbol: "X", Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase},
		{Symbol: "X", Type: TransactionSell, Quantity: 10, Price: 120, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour)},
	}
	states := ComputeHoldingStates(txs, nil)
	s := states["X"]
	if s.Quantity != 0 {
		t.Fatalf("qty = %f, want 0 after full close", s.Quantity)
	}
	if s.PMC != 0 {
		t.Fatalf("pmc = %f, want 0 after full close", s.PMC)
	}
}

func TestComputeHoldingStatesReopenAfterClose(t *testing.T) {
	t.Parallel()
	txs := []Transaction{
		{Symbol: "X", Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase},
		{Symbol: "X", Type: TransactionSell, Quantity: 10, Price: 120, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour)},
		// Re-open: PMC must reset to new buy price
		{Symbol: "X", Type: TransactionBuy, Quantity: 5, Price: 130, Fee: 2,
			AllocationType: AllocationCore, ExecutedAt: txBase.Add(2 * time.Hour)},
	}
	states := ComputeHoldingStates(txs, nil)
	s := states["X"]
	if s.Quantity != 5 {
		t.Fatalf("qty = %f, want 5", s.Quantity)
	}
	// PMC = (0 + 5*130 + 2) / 5 = 130.4
	if s.PMC != 130.4 {
		t.Fatalf("pmc = %f, want 130.4", s.PMC)
	}
	if s.AllocationType != AllocationCore {
		t.Fatalf("alloc = %q, want CORE (last buy wins)", s.AllocationType)
	}
}

func TestComputeHoldingStatesMultipleSymbols(t *testing.T) {
	t.Parallel()
	txs := []Transaction{
		{Symbol: "A", Type: TransactionBuy, Quantity: 5, Price: 100, Fee: 0,
			AllocationType: AllocationCore, ExecutedAt: txBase},
		{Symbol: "B", Type: TransactionBuy, Quantity: 3, Price: 200, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase},
	}
	states := ComputeHoldingStates(txs, nil)
	if len(states) != 2 {
		t.Fatalf("states len = %d, want 2", len(states))
	}
	if states["A"].Quantity != 5 {
		t.Fatalf("A qty = %f, want 5", states["A"].Quantity)
	}
	if states["B"].Quantity != 3 {
		t.Fatalf("B qty = %f, want 3", states["B"].Quantity)
	}
}

func TestComputeHoldingStatesOversellClampsToZero(t *testing.T) {
	t.Parallel()
	txs := []Transaction{
		{Symbol: "X", Type: TransactionBuy, Quantity: 5, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase},
		// Selling more than held — quantity must clamp to 0
		{Symbol: "X", Type: TransactionSell, Quantity: 10, Price: 110, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour)},
	}
	states := ComputeHoldingStates(txs, nil)
	s := states["X"]
	if s.Quantity != 0 {
		t.Fatalf("qty = %f, want 0 after oversell", s.Quantity)
	}
	if s.PMC != 0 {
		t.Fatalf("pmc = %f, want 0 after oversell", s.PMC)
	}
}

func TestComputeHoldingStatesEmptyInput(t *testing.T) {
	t.Parallel()
	states := ComputeHoldingStates(nil, nil)
	if len(states) != 0 {
		t.Fatalf("states len = %d, want 0", len(states))
	}
}
