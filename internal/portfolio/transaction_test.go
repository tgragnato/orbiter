//nolint:testpackage // accesses unexported holdingState type via ComputeHoldingStates
package portfolio

import (
	"math"
	"testing"
	"time"
)

//nolint:gochecknoglobals // shared test fixture used across holdings_test.go
var txBase = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

const vwceMI = "VWCE.MI"

func TestComputeHoldingStatesSingleBuy(t *testing.T) {
	t.Parallel()

	txs := []Transaction{
		{
			ID: 0, Symbol: vwceMI, Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 2,
			AllocationType: AllocationCore, ExecutedAt: txBase, Currency: "", CreatedAt: time.Time{},
		},
	}

	states := ComputeHoldingStates(txs, nil)
	state := states[vwceMI]

	if state.Quantity != 10 {
		t.Fatalf("qty = %f, want 10", state.Quantity)
	}

	// PMC = (0 + 10*100 + 2) / 10 = 100.2
	if state.PMC != 100.2 {
		t.Fatalf("pmc = %f, want 100.2", state.PMC)
	}

	if state.AllocationType != AllocationCore {
		t.Fatalf("alloc = %q, want CORE", state.AllocationType)
	}
}

func TestComputeHoldingStatesAccumulateBuys(t *testing.T) {
	t.Parallel()

	txs := []Transaction{
		{
			ID: 0, Symbol: "X", Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase, Currency: "", CreatedAt: time.Time{},
		},
		{
			ID: 0, Symbol: "X", Type: TransactionBuy, Quantity: 5, Price: 110, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour),
			Currency: "", CreatedAt: time.Time{},
		},
	}

	states := ComputeHoldingStates(txs, nil)
	state := states["X"]

	if state.Quantity != 15 {
		t.Fatalf("qty = %f, want 15", state.Quantity)
	}

	// PMC = (10*100 + 5*110) / 15 = 1550/15
	want := (10.0*100 + 5*110) / 15

	if math.Abs(state.PMC-want) > 1e-9 {
		t.Fatalf("pmc = %f, want %f", state.PMC, want)
	}
}

func TestComputeHoldingStatesPartialSell(t *testing.T) {
	t.Parallel()

	txs := []Transaction{
		{
			ID: 0, Symbol: "X", Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase, Currency: "", CreatedAt: time.Time{},
		},
		{
			ID: 0, Symbol: "X", Type: TransactionSell, Quantity: 4, Price: 120, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour),
			Currency: "", CreatedAt: time.Time{},
		},
	}

	states := ComputeHoldingStates(txs, nil)
	state := states["X"]

	if state.Quantity != 6 {
		t.Fatalf("qty = %f, want 6", state.Quantity)
	}

	// PMC unchanged on sell
	if state.PMC != 100 {
		t.Fatalf("pmc = %f, want 100 (PMC unchanged on sell)", state.PMC)
	}
}

func TestComputeHoldingStatesFullSellResetsState(t *testing.T) {
	t.Parallel()

	txs := []Transaction{
		{
			ID: 0, Symbol: "X", Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase, Currency: "", CreatedAt: time.Time{},
		},
		{
			ID: 0, Symbol: "X", Type: TransactionSell, Quantity: 10, Price: 120, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour),
			Currency: "", CreatedAt: time.Time{},
		},
	}

	states := ComputeHoldingStates(txs, nil)
	state := states["X"]

	if state.Quantity != 0 {
		t.Fatalf("qty = %f, want 0 after full close", state.Quantity)
	}

	if state.PMC != 0 {
		t.Fatalf("pmc = %f, want 0 after full close", state.PMC)
	}
}

func TestComputeHoldingStatesReopenAfterClose(t *testing.T) {
	t.Parallel()

	txs := []Transaction{
		{
			ID: 0, Symbol: "X", Type: TransactionBuy, Quantity: 10, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase, Currency: "", CreatedAt: time.Time{},
		},
		{
			ID: 0, Symbol: "X", Type: TransactionSell, Quantity: 10, Price: 120, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour),
			Currency: "", CreatedAt: time.Time{},
		},
		// Re-open: PMC must reset to new buy price
		{
			ID: 0, Symbol: "X", Type: TransactionBuy, Quantity: 5, Price: 130, Fee: 2,
			AllocationType: AllocationCore, ExecutedAt: txBase.Add(2 * time.Hour),
			Currency: "", CreatedAt: time.Time{},
		},
	}

	states := ComputeHoldingStates(txs, nil)
	state := states["X"]

	if state.Quantity != 5 {
		t.Fatalf("qty = %f, want 5", state.Quantity)
	}

	// PMC = (0 + 5*130 + 2) / 5 = 130.4
	if state.PMC != 130.4 {
		t.Fatalf("pmc = %f, want 130.4", state.PMC)
	}

	if state.AllocationType != AllocationCore {
		t.Fatalf("alloc = %q, want CORE (last buy wins)", state.AllocationType)
	}
}

func TestComputeHoldingStatesMultipleSymbols(t *testing.T) {
	t.Parallel()

	txs := []Transaction{
		{
			ID: 0, Symbol: "A", Type: TransactionBuy, Quantity: 5, Price: 100, Fee: 0,
			AllocationType: AllocationCore, ExecutedAt: txBase, Currency: "", CreatedAt: time.Time{},
		},
		{
			ID: 0, Symbol: "B", Type: TransactionBuy, Quantity: 3, Price: 200, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase, Currency: "", CreatedAt: time.Time{},
		},
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
		{
			ID: 0, Symbol: "X", Type: TransactionBuy, Quantity: 5, Price: 100, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase, Currency: "", CreatedAt: time.Time{},
		},
		// Selling more than held -- quantity must clamp to 0
		{
			ID: 0, Symbol: "X", Type: TransactionSell, Quantity: 10, Price: 110, Fee: 0,
			AllocationType: AllocationSatellite, ExecutedAt: txBase.Add(time.Hour),
			Currency: "", CreatedAt: time.Time{},
		},
	}

	states := ComputeHoldingStates(txs, nil)
	state := states["X"]

	if state.Quantity != 0 {
		t.Fatalf("qty = %f, want 0 after oversell", state.Quantity)
	}

	if state.PMC != 0 {
		t.Fatalf("pmc = %f, want 0 after oversell", state.PMC)
	}
}

func TestComputeHoldingStatesEmptyInput(t *testing.T) {
	t.Parallel()

	states := ComputeHoldingStates(nil, nil)

	if len(states) != 0 {
		t.Fatalf("states len = %d, want 0", len(states))
	}
}
