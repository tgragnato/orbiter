// Package feed provides a background worker that keeps market prices current
// by fetching EOD quotes from the configured data provider.
package feed

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio"
	"github.com/tgragnato/orbiter/internal/portfolio/analytics"
	"github.com/tgragnato/orbiter/internal/portfolio/data"
	"github.com/tgragnato/orbiter/internal/portfolio/fx"
)

// oneDay is the duration of one UTC calendar day, used for date truncation and
// price-map lookups.
const oneDay = 24 * time.Hour

// PriceStore is the subset of portfolio.TransactionStore needed by the updater.
type PriceStore interface {
	ActiveSymbols(ctx context.Context) ([]string, error)
	UpdateMarketPrice(ctx context.Context, symbol string, price float64) error
	// UpdateHoldingCurrency persists the ISO 4217 currency discovered from the
	// data provider's response. Called once per symbol per refresh cycle.
	UpdateHoldingCurrency(ctx context.Context, symbol, currency string) error
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

// NAVLister can list current holdings with their quantities and market prices.
// *portfolio.PostgresStore satisfies this interface.
type NAVLister interface {
	ListHoldings(ctx context.Context) ([]portfolio.Holding, error)
}

// NAVSnapper persists NAV checkpoints and exposes the last recorded date for
// idempotent backfill. *analytics.TWREngine satisfies this interface.
type NAVSnapper interface {
	RecordNAVSnapshot(ctx context.Context, portfolioID string, snapshotAt time.Time, nav float64) error
	// LastNAVSnapshotAt returns the most recent snapshot timestamp.
	// Returns (zero, false, nil) when no snapshots exist yet.
	LastNAVSnapshotAt(ctx context.Context, portfolioID string) (time.Time, bool, error)
}

// NAVBackfiller provides the transaction history needed to reconstruct
// historical daily NAVs. *portfolio.PostgresStore satisfies this interface.
type NAVBackfiller interface {
	AllTransactionSymbols(ctx context.Context) ([]string, error)
	ListTransactions(ctx context.Context, symbol string) ([]portfolio.Transaction, error)
}

// CashFlowRecorder persists cash flows derived from trade transactions.
// *analytics.TWREngine satisfies this interface.
type CashFlowRecorder interface {
	BackfillTransactionFlows(ctx context.Context, portfolioID string, flows []analytics.TransactionFlow) error
}

// SplitPersister persists stock split events detected in candle data.
// *portfolio.PostgresStore satisfies this interface.
type SplitPersister interface {
	UpsertSplit(ctx context.Context, symbol string, splitDate time.Time, factor float64) error
}

// WatchlistPriceStore is the persistence interface for keeping watchlist item
// prices current. *portfolio.PostgresStore satisfies this interface.
type WatchlistPriceStore interface {
	ListWatchlistSymbols(ctx context.Context) ([]string, error)
	UpdateWatchlistPrice(ctx context.Context, symbol string, price float64, currency string) error
}

// Updater fetches EOD prices from a DataProvider and writes them to the store.
type Updater struct {
	mu          sync.RWMutex
	backfilling atomic.Bool

	store            PriceStore
	divSyncer        DividendSyncer      // optional; nil disables dividend sync
	navLister        NAVLister           // optional; nil disables NAV snapshotting
	navSnapper       NAVSnapper          // optional; nil disables NAV snapshotting
	navBackfiller    NAVBackfiller       // optional; nil disables historical NAV backfill
	cashFlowRecorder CashFlowRecorder    // optional; nil disables cash flow backfill
	splitPersister   SplitPersister      // optional; nil disables split detection
	watchlistStore   WatchlistPriceStore // optional; nil disables watchlist price updates
	fxService        *fx.Service         // optional; nil disables FX conversion in NAV aggregation
	baseCurrency     string              // ISO 4217 portfolio base currency (default "EUR") — protected by mu
	portfolioID      string              // used when navSnapper is set
	provider         data.DataProvider
	interval         time.Duration
}

// New creates a price feed updater without dividend sync.
func New(store PriceStore, provider data.DataProvider, interval time.Duration) *Updater {
	return &Updater{
		mu:               sync.RWMutex{},
		backfilling:      atomic.Bool{},
		store:            store,
		divSyncer:        nil,
		navLister:        nil,
		navSnapper:       nil,
		navBackfiller:    nil,
		cashFlowRecorder: nil,
		splitPersister:   nil,
		watchlistStore:   nil,
		fxService:        nil,
		baseCurrency:     "",
		portfolioID:      "",
		provider:         provider,
		interval:         interval,
	}
}

// NewWithDividendSync creates a price feed updater that also keeps
// dividend_income_records up to date. On first Run() it performs a full
// historical backfill; subsequent refreshes process only the recent candles
// already fetched for the price update.
func NewWithDividendSync(
	store PriceStore,
	divSyncer DividendSyncer,
	provider data.DataProvider,
	interval time.Duration,
) *Updater {
	return &Updater{
		mu:               sync.RWMutex{},
		backfilling:      atomic.Bool{},
		store:            store,
		divSyncer:        divSyncer,
		navLister:        nil,
		navSnapper:       nil,
		navBackfiller:    nil,
		cashFlowRecorder: nil,
		splitPersister:   nil,
		watchlistStore:   nil,
		fxService:        nil,
		baseCurrency:     "",
		portfolioID:      "",
		provider:         provider,
		interval:         interval,
	}
}

// WithNAVSnapshot enables automatic NAV snapshotting after each price refresh.
// lister reads the current holdings (quantities × market prices), snapper
// persists the resulting total NAV, and portfolioID identifies the portfolio
// record. Calling this method on a nil Updater panics.
func (u *Updater) WithNAVSnapshot(lister NAVLister, snapper NAVSnapper, portfolioID string) *Updater {
	u.navLister = lister
	u.navSnapper = snapper
	u.portfolioID = portfolioID

	return u
}

// WithSplitPersister enables automatic split detection and persistence.
// Each candle whose SplitFactor differs from 1.0 is stored in stock_splits so
// that ComputeHoldingStates and ComputeDailyNAVs can normalise quantities.
// Calling this method on a nil Updater panics.
func (u *Updater) WithSplitPersister(persister SplitPersister) *Updater {
	u.splitPersister = persister

	return u
}

// WithCashFlowRecorder enables automatic cash flow backfill on startup.
// For each BUY transaction a DEPOSIT is recorded; for each SELL a WITHDRAWAL.
// This populates cash_flows so the TWR formula can isolate price returns from
// capital injections. Requires WithNAVBackfill to be configured (it reuses the
// same transaction list). Calling this method on a nil Updater panics.
func (u *Updater) WithCashFlowRecorder(recorder CashFlowRecorder) *Updater {
	u.cashFlowRecorder = recorder

	return u
}

// SetBaseCurrency updates the ISO 4217 base currency used for multi-currency NAV
// aggregation. Safe to call while the Updater is running. The next refresh cycle
// and any subsequent backfill will use the new value. Call TriggerBackfill
// afterwards to rebuild historical NAV snapshots in the new currency.
func (u *Updater) SetBaseCurrency(currency string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.baseCurrency = currency
}

// WithFXService enables multi-currency NAV aggregation. When set, each
// holding's NAV is converted to baseCurrency before being summed. baseCurrency
// must be a valid ISO 4217 code (e.g. "EUR"). Calling this method on a nil
// Updater panics.
func (u *Updater) WithFXService(svc *fx.Service, baseCurrency string) *Updater {
	u.fxService = svc
	u.SetBaseCurrency(baseCurrency)

	return u
}

// TriggerBackfill asynchronously re-runs the historical NAV backfill using the
// current base currency. Intended for hot-reload after SetBaseCurrency is called.
// No-op when NAVBackfiller or NAVSnapper are not configured, or when a backfill
// is already in progress.
func (u *Updater) TriggerBackfill(ctx context.Context) {
	if u.navBackfiller == nil || u.navSnapper == nil {
		return
	}

	if !u.backfilling.CompareAndSwap(false, true) {
		slog.Debug("price feed: backfill already in progress, skipping trigger")

		return
	}

	go func() {
		defer u.backfilling.Store(false)

		u.backfillNAV(ctx)
	}()
}

// WithWatchlistUpdater enables price updates for watchlist items. On every
// refresh cycle the feed fetches the latest EOD quote for each watchlist symbol
// and persists it to the watchlist table, making prices visible in the TUI.
func (u *Updater) WithWatchlistUpdater(ws WatchlistPriceStore) *Updater {
	u.watchlistStore = ws

	return u
}

// WithNAVBackfill enables one-time historical NAV backfill on startup.
// backfiller supplies the full transaction history; the backfill is skipped
// automatically for days already present in nav_snapshots, making it safe
// to call on every restart. Requires WithNAVSnapshot to also be configured.
func (u *Updater) WithNAVBackfill(backfiller NAVBackfiller) *Updater {
	u.navBackfiller = backfiller

	return u
}

// Run starts the refresh loop. It fetches prices immediately at startup, then
// repeats every interval until ctx is cancelled. When a DividendSyncer is
// configured, a full historical dividend backfill runs asynchronously before
// the first price refresh. When a NAVBackfiller is configured alongside a
// NAVSnapper, a one-time historical NAV backfill also runs asynchronously.
// Cash flow backfill is merged into backfillNAV so both use the same adjusted
// close prices from the price map — this keeps deposit amounts consistent with
// the NAV snapshots and avoids TWR distortions from retroactive price adjustments.
func (u *Updater) Run(ctx context.Context) {
	if u.divSyncer != nil {
		go u.backfillDividends(ctx)
	}

	if u.navBackfiller != nil && u.navSnapper != nil {
		u.backfilling.Store(true)

		go func() {
			defer u.backfilling.Store(false)

			u.backfillNAV(ctx)
		}()
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

// getBaseCurrency returns the current base currency in a thread-safe way.
func (u *Updater) getBaseCurrency() string {
	u.mu.RLock()
	defer u.mu.RUnlock()

	return u.baseCurrency
}

//nolint:gocognit,cyclop,funlen // orchestrates multiple optional sub-steps; extracting would obscure the flow.
func (u *Updater) refresh(ctx context.Context) {
	baseCurrency := u.getBaseCurrency()

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

	// currenciesFound collects unique (currency, baseCurrency) pairs encountered
	// during this refresh so FX rates can be synced in one pass afterwards.
	type fxPair struct{ from, to string }

	fxPairsNeeded := make(map[fxPair]struct{})

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

		err = u.store.UpdateMarketPrice(ctx, sym, price)
		if err != nil {
			slog.Error("price feed: update failed", "symbol", sym, "error", err)
		} else {
			slog.Info("price feed: updated", "symbol", sym, "price", price)
		}

		// Persist the asset currency discovered from the provider response.
		if last.Currency != "" {
			err = u.store.UpdateHoldingCurrency(ctx, sym, last.Currency)
			if err != nil {
				slog.Warn("price feed: update currency failed", "symbol", sym, "error", err)
			}

			if u.fxService != nil && baseCurrency != "" && last.Currency != baseCurrency {
				fxPairsNeeded[fxPair{from: last.Currency, to: baseCurrency}] = struct{}{}
			}
		}

		// Process any dividend events that appear in the recent candles.
		if u.divSyncer != nil {
			u.syncRecentDividends(ctx, sym, candles)
		}
	}

	// Sync today's FX rates for all currency pairs encountered during this refresh.
	if u.fxService != nil {
		for pair := range fxPairsNeeded {
			err = u.fxService.SyncRates(ctx, pair.from, pair.to, now, now)
			if err != nil {
				slog.Warn("price feed: FX sync failed", "from", pair.from, "to", pair.to, "error", err)
			}
		}
	}

	// Update prices for watchlist symbols (assets tracked for entry, not yet held).
	if u.watchlistStore != nil {
		u.refreshWatchlist(ctx)
	}

	// After all prices are refreshed, record a NAV snapshot so the TWR chart
	// accumulates data points over time.
	if u.navLister != nil && u.navSnapper != nil {
		err = u.recordNAV(ctx)
		if err != nil {
			slog.Warn("price feed: NAV snapshot failed", "error", err)
		}
	}
}

// refreshWatchlist fetches the latest EOD price for every watchlist symbol and
// persists it. Symbols that are already active holdings are skipped — their
// price is already updated by the main refresh loop above.
func (u *Updater) refreshWatchlist(ctx context.Context) {
	symbols, err := u.watchlistStore.ListWatchlistSymbols(ctx)
	if err != nil {
		slog.Warn("price feed: list watchlist symbols", "error", err)

		return
	}

	if len(symbols) == 0 {
		return
	}

	now := time.Now().UTC()
	from := now.AddDate(0, 0, -5)

	for _, sym := range symbols {
		candles, err := u.provider.GetEOD(sym, from, now)
		if err != nil {
			slog.Warn("price feed: watchlist EOD fetch failed", "symbol", sym, "error", err)

			continue
		}

		if len(candles) == 0 {
			continue
		}

		last := candles[len(candles)-1]

		price := last.AdjustedClose
		if price <= 0 {
			price = last.Close
		}

		if price <= 0 {
			slog.Warn("price feed: watchlist zero price, skipping", "symbol", sym)

			continue
		}

		err = u.watchlistStore.UpdateWatchlistPrice(ctx, sym, price, last.Currency)
		if err != nil {
			slog.Warn("price feed: update watchlist price failed", "symbol", sym, "error", err)
		} else {
			slog.Info("price feed: watchlist updated", "symbol", sym, "price", price, "currency", last.Currency)
		}
	}
}

// recordNAV computes the total portfolio NAV from current holdings and
// persists it as a TWR checkpoint. When an FX service is configured, each
// holding's NAV is converted to the portfolio base currency before aggregation
// so that the snapshot is always denominated in a single currency.
func (u *Updater) recordNAV(ctx context.Context) error {
	holdings, err := u.navLister.ListHoldings(ctx)
	if err != nil {
		return fmt.Errorf("list holdings: %w", err)
	}

	var totalNAV float64

	for _, h := range holdings {
		totalNAV += h.NAV()
	}

	if totalNAV <= 0 {
		slog.Debug("price feed: total NAV is zero, skipping snapshot")

		return nil
	}

	err = u.navSnapper.RecordNAVSnapshot(ctx, u.portfolioID, time.Now(), totalNAV)
	if err != nil {
		return fmt.Errorf("record NAV snapshot: %w", err)
	}

	return nil
}

// syncRecentDividends processes dividend events found in candles that were
// already fetched for the price update. This is a cheap check — most calls
// return immediately because CashDividend == 0 for all recent candles.
func (u *Updater) syncRecentDividends(ctx context.Context, sym string, candles []data.Candle) {
	hasDividend := false

	for _, candle := range candles {
		if candle.CashDividend > 0 {
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

	err = u.divSyncer.UpsertDividendIncomes(ctx, records)
	if err != nil {
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
		err = u.divSyncer.UpsertDividendIncomes(ctx, records)
		if err != nil {
			slog.Error("dividend backfill: upsert failed", "symbol", sym, "error", err)
		} else {
			slog.Info("dividend backfill: done", "symbol", sym, "records", len(records))
		}
	}
}

// backfillNAV reconstructs historical daily NAV snapshots from the full
// transaction and candle history. It always starts from the first transaction
// date and covers every day up to (but not including) today, relying on
// ON CONFLICT DO NOTHING in RecordSnapshot for idempotency. Starting from the
// first transaction date rather than the last recorded snapshot avoids a race
// condition where the regular refresh() call inserts today's snapshot before
// this goroutine runs, which would otherwise cause the backfill to skip all
// historical data on the very first startup.
// Runs once asynchronously at startup.
//
//nolint:gocognit,cyclop,gocyclo,funlen // complex pipeline; extracting sub-steps would obscure the algorithm.
func (u *Updater) backfillNAV(ctx context.Context) {
	baseCurrency := u.getBaseCurrency()

	allTxs, err := u.navBackfiller.ListTransactions(ctx, "")
	if err != nil {
		slog.Warn("nav backfill: list transactions", "error", err)

		return
	}

	if len(allTxs) == 0 {
		return
	}

	firstTxDate := allTxs[0].ExecutedAt.UTC().Truncate(oneDay)
	today := time.Now().UTC().Truncate(oneDay)

	// Nothing historical to backfill if the first trade is today.
	if !firstTxDate.Before(today) {
		slog.Debug("nav backfill: first trade is today, nothing to backfill", "portfolio", u.portfolioID)

		return
	}

	// Fetch historical candles for every symbol ever traded.
	symbols, err := u.navBackfiller.AllTransactionSymbols(ctx)
	if err != nil {
		slog.Warn("nav backfill: list symbols", "error", err)

		return
	}

	// priceMap: symbol → (UTC day) → adjusted-close price (in base currency when FX is available).
	// splitMap: symbol → []SplitEvent (factor != 1.0 candles).
	// symbolCurrency: symbol → ISO 4217 currency reported by the data provider.
	priceMap := make(map[string]map[time.Time]float64, len(symbols))
	splitMap := make(map[string][]portfolio.SplitEvent)

	symbolCurrency := make(map[string]string, len(symbols))

	for _, sym := range symbols {
		if ctx.Err() != nil {
			return
		}

		candles, err := u.provider.GetEOD(sym, firstTxDate, today)
		if err != nil {
			slog.Warn("nav backfill: fetch candles", "symbol", sym, "error", err)

			continue
		}

		dayMap := make(map[time.Time]float64, len(candles))

		for _, candle := range candles {
			calDay := candle.Time.UTC().Truncate(oneDay)

			price := candle.AdjustedClose
			if price <= 0 {
				price = candle.Close
			}

			if price > 0 {
				dayMap[calDay] = price
			}

			// Track the asset's quotation currency (last non-empty value wins).
			if candle.Currency != "" {
				symbolCurrency[sym] = candle.Currency
			}

			// A SplitFactor != 1.0 (and != 0) signals a split on that day.
			if candle.SplitFactor != 0 && candle.SplitFactor != 1.0 {
				splitMap[sym] = append(splitMap[sym], portfolio.SplitEvent{
					Date:   calDay,
					Factor: candle.SplitFactor,
				})

				if u.splitPersister != nil {
					pErr := u.splitPersister.UpsertSplit(ctx, sym, calDay, candle.SplitFactor)
					if pErr != nil {
						slog.Warn("nav backfill: persist split", "symbol", sym, "date", calDay, "error", pErr)
					}
				}
			}
		}

		if len(dayMap) > 0 {
			priceMap[sym] = dayMap
		}
	}

	// Multi-currency: when an FX service is available, convert each symbol's
	// prices to base currency before NAV aggregation. This keeps ComputeDailyNAVs
	// currency-agnostic — it always sums values in a single unit.
	//nolint:nestif // multi-currency FX sync and conversion are inherently nested; extracting would hurt readability.
	if u.fxService != nil && baseCurrency != "" {
		// Collect unique foreign-currency pairs to sync in one pass.
		type fxPair struct{ from, to string }

		toSync := make(map[fxPair]struct{})

		for sym, currency := range symbolCurrency {
			if currency != "" && currency != baseCurrency {
				if _, hasData := priceMap[sym]; hasData {
					toSync[fxPair{from: currency, to: baseCurrency}] = struct{}{}
				}
			}
		}

		for pair := range toSync {
			if ctx.Err() != nil {
				return
			}

			err := u.fxService.SyncRates(ctx, pair.from, pair.to, firstTxDate, today)
			if err != nil {
				slog.Warn("nav backfill: historical FX sync failed",
					"from", pair.from, "to", pair.to, "error", err)
			}
		}

		// Convert each price point in priceMap to base currency.
		for sym, dayMap := range priceMap {
			currency := symbolCurrency[sym]
			if currency == "" || currency == baseCurrency {
				continue
			}

			for calDay, price := range dayMap {
				converted, err := u.fxService.Convert(ctx, price, currency, baseCurrency, calDay)
				if err != nil {
					// Degrade gracefully: leave the unconverted local-currency price.
					slog.Debug("nav backfill: FX conversion skipped, using local price",
						"symbol", sym, "date", calDay, "error", err)

					continue
				}

				dayMap[calDay] = converted
			}
		}
	}

	// Reconstruct one NAV point per trading day in [firstTxDate, today).
	dailyNAVs := portfolio.ComputeDailyNAVs(allTxs, priceMap, splitMap, firstTxDate, today)
	if len(dailyNAVs) == 0 {
		slog.Debug("nav backfill: no data points to record", "portfolio", u.portfolioID)

		return
	}

	recorded := 0

	for _, navPoint := range dailyNAVs {
		if ctx.Err() != nil {
			return
		}

		err := u.navSnapper.RecordNAVSnapshot(ctx, u.portfolioID, navPoint.Date, navPoint.NAV)
		if err != nil {
			slog.Warn("nav backfill: record snapshot", "date", navPoint.Date, "error", err)
		} else {
			recorded++
		}
	}

	slog.Info("nav backfill: done", "portfolio", u.portfolioID, "snapshots", recorded)

	// Cash flows are computed here (not in a separate goroutine) so they use the
	// same priceMap already fetched above. This is critical for TWR correctness:
	// if deposit amounts used the actual transaction price while NAV snapshots use
	// the adjusted close price, any subsequent dividend payments would cause the
	// deposit to be larger than the NAV increase it caused, producing artificial
	// negative TWR in periods around each purchase.
	if u.cashFlowRecorder != nil {
		flows := buildAdjustedFlows(allTxs, priceMap)

		err := u.cashFlowRecorder.BackfillTransactionFlows(ctx, u.portfolioID, flows)
		if err != nil {
			slog.Error("nav backfill: cash flow persist failed", "error", err)
		} else {
			slog.Info("nav backfill: cash flows recorded", "portfolio", u.portfolioID, "count", len(flows))
		}
	}
}

// buildAdjustedFlows derives per-transaction cash flows using adjusted close
// prices from priceMap. This keeps deposit/withdrawal amounts consistent with
// the NAV snapshots (which also use adjusted prices), preventing TWR distortions
// caused by retroactive dividend adjustments.
//
//   - BUY  → DEPOSIT   (amount = qty × adjPrice + fee)
//   - SELL → WITHDRAWAL (amount = qty × adjPrice − fee, skipped when ≤ 0)
//
// occurred_at is midnight UTC of the transaction day so each flow falls inside
// the correct daily sub-period used by CalculateTWR.
func buildAdjustedFlows(
	txs []portfolio.Transaction,
	priceMap map[string]map[time.Time]float64,
) []analytics.TransactionFlow {
	flows := make([]analytics.TransactionFlow, 0, len(txs))

	for txIdx := range txs {
		transaction := &txs[txIdx]
		day := transaction.ExecutedAt.UTC().Truncate(oneDay)
		adjPrice := lookupAdjustedPrice(priceMap[transaction.Symbol], day, transaction.Price)

		switch transaction.Type {
		case portfolio.TransactionBuy:
			flows = append(flows, analytics.TransactionFlow{
				Type:       analytics.CashFlowDeposit,
				Amount:     transaction.Quantity*adjPrice + transaction.Fee,
				OccurredAt: day,
			})
		case portfolio.TransactionSell:
			amount := transaction.Quantity*adjPrice - transaction.Fee
			if amount <= 0 {
				continue
			}

			flows = append(flows, analytics.TransactionFlow{
				Type:       analytics.CashFlowWithdrawal,
				Amount:     amount,
				OccurredAt: day,
			})
		}
	}

	return flows
}

// lookupAdjustedPrice returns the adjusted close price for a symbol on the given
// UTC day. It first tries the exact day, then searches up to 3 days forward and
// backward (to cover weekends and exchange holidays), and finally falls back to
// the actual transaction price so the flow is always recorded.
func lookupAdjustedPrice(dayMap map[time.Time]float64, day time.Time, fallback float64) float64 {
	if p, ok := dayMap[day]; ok {
		return p
	}

	for i := 1; i <= 3; i++ {
		if p, ok := dayMap[day.Add(time.Duration(i)*oneDay)]; ok {
			return p
		}

		if p, ok := dayMap[day.Add(-time.Duration(i)*oneDay)]; ok {
			return p
		}
	}

	return fallback
}
