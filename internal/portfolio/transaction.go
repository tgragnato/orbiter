package portfolio

import (
	"context"
	"time"
)

// TransactionType identifies a trade execution as a purchase or a sale.
type TransactionType string

const (
	TransactionBuy  TransactionType = "BUY"
	TransactionSell TransactionType = "SELL"
)

// Transaction records a single executed asset trade.
type Transaction struct {
	ID             int64
	Symbol         string
	Type           TransactionType
	Quantity       float64
	Price          float64
	Fee            float64
	AllocationType AllocationType
	ExecutedAt     time.Time
	CreatedAt      time.Time
}

// holdingState is the running per-symbol state during transaction replay.
type holdingState struct {
	Quantity       float64
	PMC            float64
	AllocationType AllocationType
	RealizedPnL    float64
}

// ComputeHoldingStates derives per-symbol quantity and PMC from an oldest-first
// ordered transaction list using the Italian weighted-average cost method
// (Prezzo Medio di Carico):
//
//	BUY:  new_pmc = (qty×pmc + buy_qty×price + fee) / (qty + buy_qty)
//	SELL: qty -= sell_qty; if qty ≤ 0 then qty = 0 and pmc = 0
//
// Symbols with zero net quantity remain in the map; the caller decides whether
// to persist or discard them.
func ComputeHoldingStates(txs []Transaction) map[string]holdingState {
	states := make(map[string]holdingState, len(txs))
	for _, tx := range txs {
		s := states[tx.Symbol]
		switch tx.Type {
		case TransactionBuy:
			totalCost := s.Quantity*s.PMC + tx.Quantity*tx.Price + tx.Fee
			s.Quantity += tx.Quantity
			if s.Quantity > 0 {
				s.PMC = totalCost / s.Quantity
			}
			s.AllocationType = tx.AllocationType
		case TransactionSell:
			if s.PMC > 0 && s.Quantity > 0 {
				sellQty := tx.Quantity
				if sellQty > s.Quantity {
					sellQty = s.Quantity
				}
				s.RealizedPnL += sellQty*(tx.Price-s.PMC) - tx.Fee
			}
			s.Quantity -= tx.Quantity
			if s.Quantity <= 0 {
				s.Quantity = 0
				s.PMC = 0
			}
		}
		states[tx.Symbol] = s
	}
	return states
}

// TransactionStore manages the trade ledger and derived holdings state.
type TransactionStore interface {
	// AddTransaction records a trade and immediately recalculates the holding.
	AddTransaction(ctx context.Context, tx Transaction) error
	// ListTransactions returns trades ordered by executed_at; pass "" for all symbols.
	ListTransactions(ctx context.Context, symbol string) ([]Transaction, error)
	// RecalculateHoldings recomputes quantity and PMC for every symbol that has
	// at least one transaction, preserving existing market_price values.
	RecalculateHoldings(ctx context.Context) error
	// UpdateMarketPrice stores a fresh quote for the named holding.
	UpdateMarketPrice(ctx context.Context, symbol string, price float64) error
	// ActiveSymbols returns tickers of all holdings with positive quantity.
	ActiveSymbols(ctx context.Context) ([]string, error)
}
