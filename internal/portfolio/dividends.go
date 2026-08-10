package portfolio

import (
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio/data"
)

// DividendRecord is a realized dividend income record for one ex-date.
type DividendRecord struct {
	Symbol               string
	ExDate               time.Time
	Quantity             float64
	CashDividendPerShare float64
	IncomeAmount         float64
	Currency             string // ISO 4217 currency of the dividend payment
}

// QuantityAtDate replays txs (oldest-first) and returns the net quantity held
// at the close of the day before the given ex-date. Under standard equity rules,
// eligibility for a dividend is determined by the close of the prior trading day
// (i.e. you must hold before the ex-date opens, not on the ex-date itself).
func QuantityAtDate(txs []Transaction, date time.Time) float64 {
	// cutoff = last nanosecond of the day before the ex-date
	cutoff := date.UTC().Truncate(24 * time.Hour).Add(-time.Nanosecond)
	qty := 0.0
	pmc := 0.0
	for i := range txs {
		tx := txs[i]
		if tx.ExecutedAt.UTC().After(cutoff) {
			break
		}
		switch tx.Type {
		case TransactionBuy:
			totalCost := qty*pmc + tx.Quantity*tx.Price + tx.Fee
			qty += tx.Quantity
			if qty > 0 {
				pmc = totalCost / qty
			}
		case TransactionSell:
			qty -= tx.Quantity
			if qty <= 0 {
				qty = 0
				pmc = 0
			}
		}
	}
	return qty
}

// ComputeDividendIncomes computes per-ex-date dividend income by replaying txs
// to find the correct quantity held at each dividend event in candles.
// txs must be sorted oldest-first (as returned by ListTransactions).
// Only candles with CashDividend > 0 and a positive held quantity are included.
func ComputeDividendIncomes(txs []Transaction, candles []data.Candle) []DividendRecord {
	var records []DividendRecord
	for _, c := range candles {
		if c.CashDividend <= 0 {
			continue
		}
		qty := QuantityAtDate(txs, c.Time)
		if qty <= 0 {
			continue
		}
		records = append(records, DividendRecord{
			Symbol:               c.Ticker,
			ExDate:               c.Time.UTC().Truncate(24 * time.Hour), // normalise to midnight UTC
			Quantity:             qty,
			CashDividendPerShare: c.CashDividend,
			IncomeAmount:         qty * c.CashDividend,
			Currency:             c.Currency,
		})
	}
	return records
}
