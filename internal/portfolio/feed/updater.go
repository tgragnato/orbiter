// Package feed provides a background worker that keeps market prices current
// by fetching EOD quotes from the configured data provider.
package feed

import (
	"context"
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/portfolio/data"
)

// PriceStore is the subset of portfolio.TransactionStore needed by the updater.
type PriceStore interface {
	ActiveSymbols(ctx context.Context) ([]string, error)
	UpdateMarketPrice(ctx context.Context, symbol string, price float64) error
}

// DividendSyncer is the persistence interface needed to sync dividend income.
// *portfolio.PostgresStore satisfies this interface.
type DividendSyncer interface {
	// AllTransactionSymbols returns every symbol ever traded, including fully-sold
	// positions. Used by the backfill so historical dividends are computed for
	// sold assets, not just currently-held ones.
	AllTransactionSymbols(ctx context.Context) ([]string, error)
	FirstTransactionDate(ctx context.Context, symbol string) (time.Time, error)
	ListTransactions(ctx context.Context, symbol string) ([]portfolio.Transaction, error)
	// DeleteDividendIncomesBySymbol removes all records for the symbol before a
	// full recompute, ensuring post-sell stale records cannot persist.
	DeleteDividendIncomesBySymbol(ctx context.Context, symbol string) error
	UpsertDividendIncomes(ctx context.Context, records []portfolio.DividendRecord) error
}

// Updater fetches EOD prices from a DataProvider and writes them to the store.
type Updater struct {
	store     PriceStore
	divSyncer DividendSyncer // optional; nil disables dividend sync
	provider  data.DataProvider
	interval  time.Duration
}

// New creates a price feed updater without dividend sync.
func New(store PriceStore, provider data.DataProvider, interval time.Duration) *Updater {
	return &Updater{store: store, provider: provider, interval: interval}
}

// NewWithDividendSync creates a price feed updater that also keeps
// dividend_income_records up to date. On first Run() it performs a full
// historical backfill; subsequent refreshes process only the recent candles
// already fetched for the price update.
func NewWithDividendSync(store PriceStore, divSyncer DividendSyncer, provider data.DataProvider, interval time.Duration) *Updater {
	return &Updater{store: store, divSyncer: divSyncer, provider: provider, interval: interval}
}

// Run starts the refresh loop. It fetches prices immediately at startup, then
// repeats every interval until ctx is cancelled. When a DividendSyncer is
// configured, a full historical dividend backfill runs asynchronously before
// the first price refresh.
func (u *Updater) Run(ctx context.Context) {
	if u.divSyncer != nil {
		go u.backfillDividends(ctx)
	}

	u.refresh(ctx)

	ticker := time.NewTicker(u.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.refresh(ctx)
		}
	}
}

func (u *Updater) refresh(ctx context.Context) {
	symbols, err := u.store.ActiveSymbols(ctx)
	if err != nil {
		slog.Error("price feed: list active symbols", "error", err)
		return
	}
	if len(symbols) == 0 {
		return
	}

	now := time.Now().UTC()
	// Request the last 5 calendar days so we always get at least one EOD candle
	// even around weekends and public holidays.
	from := now.AddDate(0, 0, -5)

	for _, sym := range symbols {
		candles, err := u.provider.GetEOD(sym, from, now)
		if err != nil {
			slog.Warn("price feed: EOD fetch failed", "symbol", sym, "error", err)
			continue
		}
		if len(candles) == 0 {
			slog.Debug("price feed: no candles returned", "symbol", sym)
			continue
		}

		last := candles[len(candles)-1]
		price := last.AdjustedClose
		if price <= 0 {
			price = last.Close
		}
		if price <= 0 {
			slog.Warn("price feed: zero price, skipping update", "symbol", sym)
			continue
		}

		if err := u.store.UpdateMarketPrice(ctx, sym, price); err != nil {
			slog.Error("price feed: update failed", "symbol", sym, "error", err)
		} else {
			slog.Info("price feed: updated", "symbol", sym, "price", price)
		}

		// Process any dividend events that appear in the recent candles.
		if u.divSyncer != nil {
			u.syncRecentDividends(ctx, sym, candles)
		}
	}
}

// syncRecentDividends processes dividend events found in candles that were
// already fetched for the price update. This is a cheap check — most calls
// return immediately because CashDividend == 0 for all recent candles.
func (u *Updater) syncRecentDividends(ctx context.Context, sym string, candles []data.Candle) {
	hasDividend := false
	for _, c := range candles {
		if c.CashDividend > 0 {
			hasDividend = true
			break
		}
	}
	if !hasDividend {
		return
	}

	txs, err := u.divSyncer.ListTransactions(ctx, sym)
	if err != nil {
		slog.Warn("price feed: list tx for dividend sync", "symbol", sym, "error", err)
		return
	}

	records := portfolio.ComputeDividendIncomes(txs, candles)
	if len(records) == 0 {
		return
	}

	if err := u.divSyncer.UpsertDividendIncomes(ctx, records); err != nil {
		slog.Error("price feed: upsert dividends", "symbol", sym, "error", err)
	}
}

// backfillDividends fetches the full candle history for every symbol that has
// ever had a transaction (including fully-sold positions) and recomputes
// dividend income with historically correct quantities. Using AllTransactionSymbols
// rather than ActiveSymbols ensures sold assets' historical dividends are not
// lost after an app restart. Each symbol's records are replaced atomically
// (delete + insert) so that stale post-sell records cannot survive a recompute.
// Runs once asynchronously at startup.
func (u *Updater) backfillDividends(ctx context.Context) {
	symbols, err := u.divSyncer.AllTransactionSymbols(ctx)
	if err != nil {
		slog.Error("dividend backfill: list symbols", "error", err)
		return
	}

	now := time.Now().UTC()
	for _, sym := range symbols {
		if ctx.Err() != nil {
			return
		}

		firstDate, err := u.divSyncer.FirstTransactionDate(ctx, sym)
		if err != nil {
			slog.Warn("dividend backfill: first tx date", "symbol", sym, "error", err)
			continue
		}

		candles, err := u.provider.GetEOD(sym, firstDate, now)
		if err != nil {
			slog.Warn("dividend backfill: fetch candles", "symbol", sym, "error", err)
			continue
		}

		txs, err := u.divSyncer.ListTransactions(ctx, sym)
		if err != nil {
			slog.Warn("dividend backfill: list transactions", "symbol", sym, "error", err)
			continue
		}

		records := portfolio.ComputeDividendIncomes(txs, candles)
		if len(records) == 0 {
			slog.Debug("dividend backfill: no dividends found", "symbol", sym)
			continue
		}
		// Upsert only — do not delete first. Stale post-sell records are removed
		// transactionally by cleanupStaleDividendRecords inside recalculateSymbol
		// whenever a transaction is added, edited, or deleted.
		if err := u.divSyncer.UpsertDividendIncomes(ctx, records); err != nil {
			slog.Error("dividend backfill: upsert failed", "symbol", sym, "error", err)
		} else {
			slog.Info("dividend backfill: done", "symbol", sym, "records", len(records))
		}
	}
}
