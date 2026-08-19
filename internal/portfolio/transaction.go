package portfolio

import (
	"context"
	"sort"
	"time"
)

// oneDay is 24 hours, used wherever a UTC day-truncation step-size is needed.
const oneDay = 24 * time.Hour

// SplitEvent records a stock split on a given date.
type SplitEvent struct {
	Date   time.Time
	Factor float64 // e.g. 2.0 for a 2-for-1 split
}

// CumulativeSplitFactor returns the product of all split factors whose Date
// falls strictly after from and no later than to. Pass a nil or empty slice to
// get 1.0 (no adjustment). Use this to convert a pre-split transaction quantity
// to its post-split equivalent: adjQty = tx.Quantity * CumulativeSplitFactor(...).
func CumulativeSplitFactor(splits []SplitEvent, from, to time.Time) float64 {
	factor := 1.0

	for _, splitEvent := range splits {
		if splitEvent.Date.After(from) && !splitEvent.Date.After(to) {
			factor *= splitEvent.Factor
		}
	}

	return factor
}

// TransactionType identifies a trade execution as a purchase or a sale.
type TransactionType string

const (
	// TransactionBuy identifies a purchase execution.
	TransactionBuy TransactionType = "BUY"
	// TransactionSell identifies a sale execution.
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
	Currency       string // ISO 4217 quotation currency at execution time
	ExecutedAt     time.Time
	CreatedAt      time.Time
}

// holdingState is the running per-symbol state during transaction replay.
type holdingState struct {
	Quantity       float64
	PMC            float64
	AllocationType AllocationType
	RealizedPnL    float64
	Currency       string // ISO 4217 currency; tracks the most recent transaction's currency
}

// ComputeHoldingStates derives per-symbol quantity and PMC from an oldest-first
// ordered transaction list using the Italian weighted-average cost method
// (Prezzo Medio di Carico):
//
//	BUY:  new_pmc = (qty×pmc + buy_qty×price + fee) / (qty + buy_qty)
//	SELL: qty -= sell_qty; if qty <= 0 then qty = 0 and pmc = 0
//
// splitMap optionally supplies per-symbol []SplitEvent. Each transaction is
// normalised to current (post-split) share count before replay: BUY quantities
// are multiplied by the cumulative factor of all splits that occurred after the
// transaction date, and prices are divided by the same factor so that the total
// cost (and realised PnL) is unchanged. Fees are absolute amounts and are never
// adjusted. Pass nil to skip split adjustment.
//
// Symbols with zero net quantity remain in the map; the caller decides whether
// to persist or discard them.
//
//nolint:cyclop // replay logic requires iterating all cases; extracting helpers would obscure the algorithm
func ComputeHoldingStates(txs []Transaction, splitMap map[string][]SplitEvent) map[string]holdingState {
	// Determine the normalisation epoch: latest executed_at in the slice, or
	// now when the slice is empty. All split factors are evaluated up to this date.
	var latest time.Time

	for i := range txs {
		if txs[i].ExecutedAt.After(latest) {
			latest = txs[i].ExecutedAt
		}
	}

	if latest.IsZero() {
		latest = time.Now().UTC()
	}

	states := make(map[string]holdingState, len(txs))

	for i := range txs {
		txn := txs[i]

		// Normalise quantity and price to post-split terms.
		factor := CumulativeSplitFactor(splitMap[txn.Symbol], txn.ExecutedAt, latest)
		adjQty := txn.Quantity * factor
		adjPrice := txn.Price / factor // fee is absolute, not per-share -- unchanged

		state := states[txn.Symbol]

		switch txn.Type {
		case TransactionBuy:
			totalCost := state.Quantity*state.PMC + adjQty*adjPrice + txn.Fee

			state.Quantity += adjQty

			if state.Quantity > 0 {
				state.PMC = totalCost / state.Quantity
			}

			state.AllocationType = txn.AllocationType

			if txn.Currency != "" {
				state.Currency = txn.Currency
			}
		case TransactionSell:
			if state.PMC > 0 && state.Quantity > 0 {
				sellQty := adjQty

				if sellQty > state.Quantity {
					sellQty = state.Quantity
				}

				state.RealizedPnL += sellQty*(adjPrice-state.PMC) - txn.Fee
			}

			state.Quantity -= adjQty

			if state.Quantity <= 0 {
				state.Quantity = 0
				state.PMC = 0
			}
		}

		states[txn.Symbol] = state
	}

	return states
}

// DailyNAV is one historical NAV data point used for TWR backfill.
type DailyNAV struct {
	Date time.Time
	NAV  float64
}

// ComputeDailyNAVs reconstructs historical total portfolio NAV for every trading
// day that appears in priceMap and falls within [startDate, endDate).
//
// txs must be ordered by ExecutedAt ASC (as returned by ListTransactions).
// priceMap is keyed by symbol -> (UTC day-truncated date) -> close price.
// splitMap is keyed by symbol -> []SplitEvent; pass nil if no splits are known.
//
// Splits are applied on their exact date before that day's transactions are
// processed, so quantities are always in the same unit as the day's close price
// (Yahoo AdjustedClose is already split-adjusted from the present backwards, so
// quantities must mirror that adjustment going forward).
//
// Prices are forward-filled: if a symbol has no price on a given day (holiday,
// exchange closed, etc.) the most recent prior price is used. This prevents
// artificial NAV collapses -- and the wild TWR swings they cause -- on days when
// some exchanges are closed while others are open.
//
// The function replays transactions incrementally so the run time is
// O(trading-days x symbols), not O(trading-days x transactions).
//
//nolint:cyclop,gocognit,funlen // NAV reconstruction algorithm is inherently complex;
// refactoring would obscure correctness.
func ComputeDailyNAVs(
	txs []Transaction,
	priceMap map[string]map[time.Time]float64,
	splitMap map[string][]SplitEvent,
	startDate, endDate time.Time,
) []DailyNAV {
	// Collect all unique trading days from priceMap that fall in range.
	daySet := make(map[time.Time]struct{})

	for _, dayMap := range priceMap {
		for dayKey := range dayMap {
			if !dayKey.Before(startDate) && dayKey.Before(endDate) {
				daySet[dayKey] = struct{}{}
			}
		}
	}

	if len(daySet) == 0 {
		return nil
	}

	days := make([]time.Time, 0, len(daySet))

	for dayKey := range daySet {
		days = append(days, dayKey)
	}

	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })

	// Replay transactions incrementally, advancing a pointer through the
	// sorted slice so each tx is processed exactly once.
	states := make(map[string]holdingState)
	txIdx := 0

	// lastPrice holds the most recently seen price for each symbol and is
	// updated on every day that has actual data. On days without a quote the
	// carried-forward value is used so that exchange-holiday gaps do not
	// create artificial zero-NAV days.
	lastPrice := make(map[string]float64, len(priceMap))

	result := make([]DailyNAV, 0, len(days))

	for _, day := range days {
		// Apply any splits that occur on this day before processing transactions.
		// Quantities are multiplied by the split factor; prices in the priceMap
		// (AdjustedClose) are already adjusted retroactively by Yahoo, so no price
		// change is needed here -- only the tracked quantity must grow.
		for sym, events := range splitMap {
			for _, ev := range events {
				if ev.Date.Equal(day) {
					if st, ok := states[sym]; ok {
						st.Quantity *= ev.Factor
						states[sym] = st
					}
				}
			}
		}

		dayEnd := day.Add(oneDay)

		for txIdx < len(txs) && txs[txIdx].ExecutedAt.Before(dayEnd) {
			txn := txs[txIdx]
			state := states[txn.Symbol]

			switch txn.Type {
			case TransactionBuy:
				totalCost := state.Quantity*state.PMC + txn.Quantity*txn.Price + txn.Fee

				state.Quantity += txn.Quantity

				if state.Quantity > 0 {
					state.PMC = totalCost / state.Quantity
				}
			case TransactionSell:
				state.Quantity -= txn.Quantity

				if state.Quantity <= 0 {
					state.Quantity = 0
					state.PMC = 0
				}
			}

			states[txn.Symbol] = state
			txIdx++
		}

		// Refresh lastPrice for any symbol that has a quote today.
		for sym, dayMap := range priceMap {
			if price, ok := dayMap[day]; ok {
				lastPrice[sym] = price
			}
		}

		var nav float64

		for sym, state := range states {
			if state.Quantity <= 0 {
				continue
			}

			if price := lastPrice[sym]; price > 0 {
				nav += state.Quantity * price
			}
		}

		if nav > 0 {
			result = append(result, DailyNAV{Date: day, NAV: nav})
		}
	}

	return result
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
