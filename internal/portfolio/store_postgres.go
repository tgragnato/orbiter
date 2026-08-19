package portfolio

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Sentinel errors for holding lookups.
var (
	ErrHoldingNotFound       = errors.New("holding not found")
	ErrHoldingSymbolNotFound = errors.New("holding symbol not found")
)

const defaultCurrency = "EUR"

// HoldingsStore is the persistence contract used by the holdings TUI.
type HoldingsStore interface {
	ListHoldings(ctx context.Context) ([]Holding, error)
	ToggleAllocation(ctx context.Context, id int64) error
	ToggleTAAEnabled(ctx context.Context, symbol string) error
	// TotalRealizedPnL computes cumulative realized profit/loss from transaction history.
	TotalRealizedPnL(ctx context.Context) (float64, error)
}

// PostgresStore persists holdings in PostgreSQL.
type PostgresStore struct {
	db          *sql.DB
	portfolioID string // optional; enables live cash-flow recording after AddTransaction
}

// NewPostgresStore creates a holdings store bound to PostgreSQL.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db, portfolioID: ""}
}

// WithPortfolioID enables immediate cash-flow recording after AddTransaction.
// When set, each new BUY/SELL appends an asset='AUTO' row to cash_flows using
// tx.Price so TWR stays approximately correct within the current session.
// The next startup's backfill replaces these rows with adjusted-close prices.
func (s *PostgresStore) WithPortfolioID(portfolioID string) *PostgresStore {
	s.portfolioID = portfolioID

	return s
}

// ListHoldings returns all holdings for the unified table view.
func (s *PostgresStore) ListHoldings(ctx context.Context) ([]Holding, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, symbol, quantity, market_price, pmc, allocation_type, taa_enabled, currency
		FROM holdings
		ORDER BY symbol, id
	`)
	if err != nil {
		return nil, fmt.Errorf("list holdings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	holdings := make([]Holding, 0)

	for rows.Next() {
		var holding Holding

		var allocation string

		err = rows.Scan(
			&holding.ID, &holding.Symbol, &holding.Quantity, &holding.MarketPrice,
			&holding.PMC, &allocation, &holding.TAAEnabled, &holding.Currency,
		)
		if err != nil {
			return nil, fmt.Errorf("scan holding: %w", err)
		}

		holding.AllocationType = parseAllocationType(allocation)
		holdings = append(holdings, holding)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate holdings: %w", err)
	}

	return holdings, nil
}

// ToggleAllocation swaps one holding allocation between CORE and SATELLITE.
func (s *PostgresStore) ToggleAllocation(ctx context.Context, holdingID int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE holdings
		SET allocation_type = CASE allocation_type
			WHEN 'CORE' THEN 'SATELLITE'
			ELSE 'CORE'
		END,
		updated_at = NOW()
		WHERE id = $1
	`, holdingID)
	if err != nil {
		return fmt.Errorf("toggle allocation: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("toggle allocation rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("holding id %d: %w", holdingID, ErrHoldingNotFound)
	}

	return nil
}

// ToggleTAAEnabled flips the taa_enabled flag for all rows with the given symbol.
func (s *PostgresStore) ToggleTAAEnabled(ctx context.Context, symbol string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE holdings
		SET taa_enabled = NOT taa_enabled,
		    updated_at  = NOW()
		WHERE symbol = $1
	`, symbol)
	if err != nil {
		return fmt.Errorf("toggle taa enabled: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("toggle taa enabled rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("holding symbol %q: %w", symbol, ErrHoldingSymbolNotFound)
	}

	return nil
}

// TotalRealizedPnL computes cumulative realized profit/loss by replaying the
// full transaction history and summing per-symbol realized gains/losses.
func (s *PostgresStore) TotalRealizedPnL(ctx context.Context) (float64, error) {
	txs, err := s.ListTransactions(ctx, "")
	if err != nil {
		return 0, err
	}

	states := ComputeHoldingStates(txs, nil)
	total := 0.0

	for _, state := range states {
		total += state.RealizedPnL
	}

	return total, nil
}

// AddTransaction inserts a trade record and immediately recalculates only the
// affected symbol's holding quantity and PMC from its full transaction history.
// When WithPortfolioID has been set, it also appends a cash_flows row so TWR
// stays approximately correct within the current session without waiting for the
// next startup backfill.
func (s *PostgresStore) AddTransaction(ctx context.Context, txn Transaction) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO transactions
			(symbol, transaction_type, quantity, price, fee, allocation_type, currency, executed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, txn.Symbol, string(txn.Type), txn.Quantity, txn.Price, txn.Fee,
		string(txn.AllocationType), txn.Currency, txn.ExecutedAt)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}

	err = s.recalculateSymbol(ctx, txn.Symbol)
	if err != nil {
		return err
	}

	if s.portfolioID != "" {
		err = s.recordLiveFlow(ctx, txn)
		if err != nil {
			// Non-fatal: TWR will be corrected on next startup backfill.
			slog.Warn("portfolio: live cash-flow record failed", "symbol", txn.Symbol, "error", err)
		}
	}

	return nil
}

// UpdateTransaction overwrites a trade record identified by txn.ID and
// recalculates the affected holding(s). If the symbol changes, both the old
// and the new symbol's holding are recalculated.
func (s *PostgresStore) UpdateTransaction(ctx context.Context, txn Transaction) error {
	var oldSymbol string

	_ = s.db.QueryRowContext(ctx, `SELECT symbol FROM transactions WHERE id = $1`, txn.ID).Scan(&oldSymbol)

	_, err := s.db.ExecContext(ctx, `
		UPDATE transactions
		SET symbol = $1, transaction_type = $2, quantity = $3, price = $4,
		    fee = $5, allocation_type = $6, executed_at = $7
		WHERE id = $8
	`, txn.Symbol, string(txn.Type), txn.Quantity, txn.Price, txn.Fee,
		string(txn.AllocationType), txn.ExecutedAt, txn.ID)
	if err != nil {
		return fmt.Errorf("update transaction %d: %w", txn.ID, err)
	}

	err = s.recalculateSymbol(ctx, txn.Symbol)
	if err != nil {
		return err
	}

	if oldSymbol != "" && oldSymbol != txn.Symbol {
		return s.recalculateSymbol(ctx, oldSymbol)
	}

	return nil
}

// DeleteTransaction removes a trade record and recalculates the affected holding.
// If the symbol had only this one transaction, the holding is removed entirely.
func (s *PostgresStore) DeleteTransaction(ctx context.Context, txnID int64) error {
	var symbol string

	err := s.db.QueryRowContext(ctx, `SELECT symbol FROM transactions WHERE id = $1`, txnID).Scan(&symbol)
	if err != nil {
		return fmt.Errorf("find transaction %d: %w", txnID, err)
	}

	_, err = s.db.ExecContext(ctx, `DELETE FROM transactions WHERE id = $1`, txnID)
	if err != nil {
		return fmt.Errorf("delete transaction %d: %w", txnID, err)
	}

	return s.recalculateSymbol(ctx, symbol)
}

// ListTransactions returns trade records ordered by execution time.
// Pass an empty symbol to list across all symbols.
func (s *PostgresStore) ListTransactions(ctx context.Context, symbol string) ([]Transaction, error) {
	query := `
		SELECT id, symbol, transaction_type, quantity, price, fee,
		       allocation_type, currency, executed_at, created_at
		FROM transactions
	`

	var args []any

	if symbol != "" {
		query += " WHERE symbol = $1"

		args = append(args, symbol)
	}

	query += " ORDER BY executed_at ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var txs []Transaction

	for rows.Next() {
		var transactionRecord Transaction

		var txType, allocType string

		err = rows.Scan(
			&transactionRecord.ID, &transactionRecord.Symbol, &txType, &transactionRecord.Quantity,
			&transactionRecord.Price, &transactionRecord.Fee,
			&allocType, &transactionRecord.Currency, &transactionRecord.ExecutedAt,
			&transactionRecord.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}

		transactionRecord.Type = TransactionType(strings.ToUpper(txType))
		transactionRecord.AllocationType = parseAllocationType(allocType)
		txs = append(txs, transactionRecord)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate transactions: %w", err)
	}

	return txs, nil
}

// RecalculateHoldings recomputes quantity and PMC for every symbol that has at
// least one transaction, using the Italian weighted-average cost method. Existing
// market_price and taa_enabled values are preserved; closed positions keep qty=0.
//
//nolint:cyclop,funlen // SQL transaction logic requires multiple sequential steps across all symbols
func (s *PostgresStore) RecalculateHoldings(ctx context.Context) error {
	txs, err := s.ListTransactions(ctx, "")
	if err != nil {
		return err
	}

	if len(txs) == 0 {
		return nil
	}

	splitMap, err := s.listAllSplits(ctx)
	if err != nil {
		return fmt.Errorf("list all splits: %w", err)
	}

	states := ComputeHoldingStates(txs, splitMap)

	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recalculate transaction: %w", err)
	}
	defer dbTx.Rollback() //nolint:errcheck // rollback is a no-op after Commit; the Commit error is the one that matters

	for symbol, state := range states {
		oldTAAEnabled := true

		var oldPrice float64

		var oldCurrency string

		_ = dbTx.QueryRowContext(ctx,
			`SELECT COALESCE(market_price, 0), COALESCE(taa_enabled, true), `+
				`COALESCE(currency, 'EUR') FROM holdings WHERE symbol = $1 LIMIT 1`,
			symbol,
		).Scan(&oldPrice, &oldTAAEnabled, &oldCurrency)

		currency := state.Currency

		if currency == "" {
			currency = oldCurrency
		}

		if currency == "" {
			currency = defaultCurrency
		}

		_, err = dbTx.ExecContext(ctx,
			`DELETE FROM holdings WHERE symbol = $1`, symbol,
		)
		if err != nil {
			return fmt.Errorf("delete holdings for %s: %w", symbol, err)
		}

		_, err = dbTx.ExecContext(ctx, `
			INSERT INTO holdings
				(symbol, quantity, market_price, pmc, allocation_type, taa_enabled, currency, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, symbol, state.Quantity, oldPrice, state.PMC,
			string(state.AllocationType), oldTAAEnabled, currency, time.Now().UTC(),
		)
		if err != nil {
			return fmt.Errorf("insert holding for %s: %w", symbol, err)
		}
	}

	err = dbTx.Commit()
	if err != nil {
		return fmt.Errorf("commit recalculate holdings: %w", err)
	}

	return nil
}

// UpdateMarketPrice stores a fresh market quote for the named holding.
func (s *PostgresStore) UpdateMarketPrice(ctx context.Context, symbol string, price float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE holdings SET market_price = $1, updated_at = NOW() WHERE symbol = $2`,
		price, symbol,
	)
	if err != nil {
		return fmt.Errorf("update market price %s: %w", symbol, err)
	}

	return nil
}

// FirstTransactionDate returns the execution date of the earliest transaction
// for the given symbol, used to determine the start of the candle history needed
// for dividend backfill.
func (s *PostgresStore) FirstTransactionDate(ctx context.Context, symbol string) (time.Time, error) {
	var firstDate time.Time

	err := s.db.QueryRowContext(ctx,
		`SELECT MIN(executed_at) FROM transactions WHERE symbol = $1`, symbol,
	).Scan(&firstDate)
	if err != nil {
		return time.Time{}, fmt.Errorf("first transaction date for %s: %w", symbol, err)
	}

	return firstDate.UTC(), nil
}

// UpsertDividendIncomes inserts or updates dividend income records.
// Records are keyed on (symbol, ex_date); existing rows are overwritten so
// that a re-run with updated quantities always reflects the current position.
func (s *PostgresStore) UpsertDividendIncomes(ctx context.Context, records []DividendRecord) error {
	for _, record := range records {
		currency := record.Currency

		if currency == "" {
			currency = defaultCurrency
		}

		_, err := s.db.ExecContext(ctx, `
			INSERT INTO dividend_income_records
				(symbol, ex_date, quantity, cash_dividend_per_share, income_amount, currency)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (symbol, ex_date) DO UPDATE SET
				quantity                = EXCLUDED.quantity,
				cash_dividend_per_share = EXCLUDED.cash_dividend_per_share,
				income_amount           = EXCLUDED.income_amount,
				currency                = EXCLUDED.currency
		`, record.Symbol, record.ExDate, record.Quantity,
			record.CashDividendPerShare, record.IncomeAmount, currency)
		if err != nil {
			return fmt.Errorf(
				"upsert dividend income %s %s: %w",
				record.Symbol, record.ExDate.Format("2006-01-02"), err,
			)
		}
	}

	return nil
}

// TotalDividendIncome returns the sum of all dividend income ever received.
// Records only exist for dates when a position was actually held (QuantityAtDate > 0),
// so no filtering by current holdings quantity is needed.
func (s *PostgresStore) TotalDividendIncome(ctx context.Context) (float64, error) {
	var total float64

	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(income_amount), 0) FROM dividend_income_records
	`).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("total dividend income: %w", err)
	}

	return total, nil
}

// ListDividendIncome returns all dividend income records, ordered by ex-date
// descending. Records only exist for dates when the position was actually held,
// so no filtering by current holdings quantity is needed.
func (s *PostgresStore) ListDividendIncome(ctx context.Context) ([]DividendRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT symbol, ex_date, quantity, cash_dividend_per_share, income_amount, currency
		FROM dividend_income_records
		ORDER BY ex_date DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list dividend income: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []DividendRecord

	for rows.Next() {
		var record DividendRecord

		err = rows.Scan(
			&record.Symbol, &record.ExDate, &record.Quantity,
			&record.CashDividendPerShare, &record.IncomeAmount, &record.Currency,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dividend income: %w", err)
		}

		records = append(records, record)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate dividend income: %w", err)
	}

	return records, nil
}

// ActiveSymbols returns the ticker symbols of all holdings with positive quantity.
func (s *PostgresStore) ActiveSymbols(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT symbol FROM holdings WHERE quantity > 0 ORDER BY symbol`,
	)
	if err != nil {
		return nil, fmt.Errorf("active symbols: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var symbols []string

	for rows.Next() {
		var sym string

		err = rows.Scan(&sym)
		if err != nil {
			return nil, fmt.Errorf("scan symbol: %w", err)
		}

		symbols = append(symbols, sym)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate active symbols: %w", err)
	}

	return symbols, nil
}

// AllTransactionSymbols returns every symbol that has at least one transaction
// record, including fully-sold positions. Used by the dividend backfill to
// ensure historical records are computed for all ever-held assets.
func (s *PostgresStore) AllTransactionSymbols(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT symbol FROM transactions ORDER BY symbol`,
	)
	if err != nil {
		return nil, fmt.Errorf("all transaction symbols: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var symbols []string

	for rows.Next() {
		var sym string

		err = rows.Scan(&sym)
		if err != nil {
			return nil, fmt.Errorf("scan symbol: %w", err)
		}

		symbols = append(symbols, sym)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate transaction symbols: %w", err)
	}

	return symbols, nil
}

// DeleteDividendIncomesBySymbol removes all dividend income records for the
// given symbol. Used by the backfill to perform a clean replace rather than
// a blind upsert that could leave stale post-sell records behind.
func (s *PostgresStore) DeleteDividendIncomesBySymbol(ctx context.Context, symbol string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM dividend_income_records WHERE symbol = $1`, symbol)
	if err != nil {
		return fmt.Errorf("delete dividend incomes for %s: %w", symbol, err)
	}

	return nil
}

// UpsertSplit persists a split event for the given symbol. It is idempotent:
// re-inserting the same (symbol, split_date) with the same factor is a no-op.
func (s *PostgresStore) UpsertSplit(ctx context.Context, symbol string, splitDate time.Time, factor float64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stock_splits (symbol, split_date, factor)
		VALUES ($1, $2, $3)
		ON CONFLICT (symbol, split_date) DO UPDATE SET factor = EXCLUDED.factor
	`, symbol, splitDate.UTC().Truncate(oneDay), factor)
	if err != nil {
		return fmt.Errorf("upsert split %s %s: %w", symbol, splitDate.Format("2006-01-02"), err)
	}

	return nil
}

// ListWatchlist returns all watchlist items ordered by creation date descending.
func (s *PostgresStore) ListWatchlist(ctx context.Context) ([]WatchlistItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, symbol, market_price, currency, created_at FROM watchlist ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list watchlist: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var items []WatchlistItem

	for rows.Next() {
		var item WatchlistItem

		err = rows.Scan(&item.ID, &item.Symbol, &item.MarketPrice, &item.Currency, &item.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan watchlist item: %w", err)
		}

		item.CreatedAt = item.CreatedAt.UTC()
		items = append(items, item)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate watchlist: %w", err)
	}

	return items, nil
}

// UpdateWatchlistPrice stores the latest EOD market price and quotation currency
// for a watchlist item. It is a no-op when the symbol is not in the watchlist.
func (s *PostgresStore) UpdateWatchlistPrice(ctx context.Context, symbol string, price float64, currency string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE watchlist SET market_price = $1, currency = $2 WHERE symbol = $3`,
		price, currency, symbol,
	)
	if err != nil {
		return fmt.Errorf("update watchlist price %s: %w", symbol, err)
	}

	return nil
}

// AddToWatchlist inserts a symbol into the watchlist. Duplicate symbols are
// silently ignored (ON CONFLICT DO NOTHING).
func (s *PostgresStore) AddToWatchlist(ctx context.Context, symbol string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO watchlist (symbol) VALUES ($1) ON CONFLICT (symbol) DO NOTHING`,
		symbol,
	)
	if err != nil {
		return fmt.Errorf("add to watchlist %s: %w", symbol, err)
	}

	return nil
}

// RemoveFromWatchlist deletes a symbol from the watchlist. It is a no-op if
// the symbol is not present.
func (s *PostgresStore) RemoveFromWatchlist(ctx context.Context, symbol string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM watchlist WHERE symbol = $1`,
		symbol,
	)
	if err != nil {
		return fmt.Errorf("remove from watchlist %s: %w", symbol, err)
	}

	return nil
}

// ListWatchlistSymbols returns the ticker strings for every watchlist entry.
// Used by the featurizer to include unowned assets in ML conviction scoring.
func (s *PostgresStore) ListWatchlistSymbols(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT symbol FROM watchlist ORDER BY symbol`,
	)
	if err != nil {
		return nil, fmt.Errorf("list watchlist symbols: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var symbols []string

	for rows.Next() {
		var sym string

		err = rows.Scan(&sym)
		if err != nil {
			return nil, fmt.Errorf("scan watchlist symbol: %w", err)
		}

		symbols = append(symbols, sym)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate watchlist symbols: %w", err)
	}

	return symbols, nil
}

// UpdateHoldingCurrency sets the ISO 4217 currency for the holding identified
// by symbol. Called by the feed updater after each price fetch to persist the
// currency parsed from the data provider's response.
func (s *PostgresStore) UpdateHoldingCurrency(ctx context.Context, symbol, currency string) error {
	if currency == "" {
		return nil
	}

	_, err := s.db.ExecContext(ctx,
		`UPDATE holdings SET currency = $1, updated_at = NOW() WHERE symbol = $2`,
		currency, symbol,
	)
	if err != nil {
		return fmt.Errorf("update holding currency %s: %w", symbol, err)
	}

	return nil
}

// cleanupStaleDividendRecords deletes any dividend_income_records for the given
// symbol whose ex_date falls in a period when the quantity held was zero (as
// determined by replaying the current transaction history). This keeps the
// table consistent whenever transactions are added, edited, or deleted.
func (s *PostgresStore) cleanupStaleDividendRecords(ctx context.Context, symbol string, txs []Transaction) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT ex_date FROM dividend_income_records WHERE symbol = $1`, symbol)
	if err != nil {
		return fmt.Errorf("list dividend dates for cleanup %s: %w", symbol, err)
	}
	defer func() { _ = rows.Close() }()

	var toDelete []time.Time

	for rows.Next() {
		var exDate time.Time

		err = rows.Scan(&exDate)
		if err != nil {
			return fmt.Errorf("scan ex_date for cleanup: %w", err)
		}

		if QuantityAtDate(txs, exDate) <= 0 {
			toDelete = append(toDelete, exDate)
		}
	}

	err = rows.Err()
	if err != nil {
		return fmt.Errorf("iterate dividend dates for cleanup: %w", err)
	}

	for _, deleteDate := range toDelete {
		_, err = s.db.ExecContext(ctx,
			`DELETE FROM dividend_income_records WHERE symbol = $1 AND ex_date = $2`,
			symbol, deleteDate)
		if err != nil {
			return fmt.Errorf(
				"delete stale dividend %s %s: %w", symbol, deleteDate.Format("2006-01-02"), err,
			)
		}
	}

	return nil
}

// listAllSplits returns every recorded split event keyed by symbol. Used by
// RecalculateHoldings to normalise the full transaction set in one pass.
func (s *PostgresStore) listAllSplits(ctx context.Context) (map[string][]SplitEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT symbol, split_date, factor FROM stock_splits ORDER BY symbol, split_date ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all splits: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string][]SplitEvent)

	for rows.Next() {
		var sym string

		var splitEvt SplitEvent

		err = rows.Scan(&sym, &splitEvt.Date, &splitEvt.Factor)
		if err != nil {
			return nil, fmt.Errorf("scan split row: %w", err)
		}

		splitEvt.Date = splitEvt.Date.UTC()
		out[sym] = append(out[sym], splitEvt)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate splits: %w", err)
	}

	return out, nil
}

// listSplitsForSymbol returns all recorded split events for one symbol, ordered
// by split_date ASC. Returns an empty slice (not nil) when none exist.
func (s *PostgresStore) listSplitsForSymbol(ctx context.Context, symbol string) ([]SplitEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT split_date, factor FROM stock_splits WHERE symbol = $1 ORDER BY split_date ASC`,
		symbol,
	)
	if err != nil {
		return nil, fmt.Errorf("list splits for %s: %w", symbol, err)
	}
	defer func() { _ = rows.Close() }()

	splits := make([]SplitEvent, 0)

	for rows.Next() {
		var splitEvt SplitEvent

		err = rows.Scan(&splitEvt.Date, &splitEvt.Factor)
		if err != nil {
			return nil, fmt.Errorf("scan split event: %w", err)
		}

		splitEvt.Date = splitEvt.Date.UTC()
		splits = append(splits, splitEvt)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate splits for symbol: %w", err)
	}

	return splits, nil
}

func parseAllocationType(value string) AllocationType {
	upper := strings.ToUpper(strings.TrimSpace(value))

	if upper == string(AllocationCore) {
		return AllocationCore
	}

	return AllocationSatellite
}

// recordLiveFlow inserts a single cash-flow row for a transaction executed in
// the current session. It uses txn.Price (not Yahoo AdjustedClose) because the
// adjusted price for today's candle is not yet available. The next startup
// backfill will replace all asset='AUTO' rows with adjusted-price equivalents,
// so this is only an intra-session approximation.
func (s *PostgresStore) recordLiveFlow(ctx context.Context, txn Transaction) error {
	var flowType string

	var amount float64

	day := txn.ExecutedAt.UTC().Truncate(oneDay)

	switch txn.Type {
	case TransactionBuy:
		flowType = "DEPOSIT"
		amount = txn.Quantity*txn.Price + txn.Fee
	case TransactionSell:
		flowType = "WITHDRAWAL"
		amount = txn.Quantity*txn.Price - txn.Fee

		if amount <= 0 {
			return nil
		}
	default:
		return nil
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cash_flows (portfolio_id, flow_type, amount, asset, currency, occurred_at)
		VALUES ($1, $2, $3, 'AUTO', $4, $5)
	`, s.portfolioID, flowType, amount, txn.Currency, day)
	if err != nil {
		return fmt.Errorf("record live flow: %w", err)
	}

	return nil
}

// recalculateSymbol recomputes quantity and PMC for a single symbol by replaying
// its full transaction history. It preserves the existing market_price and
// taa_enabled flag; closed positions (zero net quantity) are kept with qty=0.
//
//nolint:cyclop,funlen // SQL transaction logic requires multiple sequential steps; extracting helpers adds indirection
func (s *PostgresStore) recalculateSymbol(ctx context.Context, symbol string) error {
	txs, err := s.ListTransactions(ctx, symbol)
	if err != nil {
		return err
	}

	// No transactions remain — remove the holding and all dividend records entirely.
	if len(txs) == 0 {
		_, err = s.db.ExecContext(ctx, `DELETE FROM holdings WHERE symbol = $1`, symbol)
		if err != nil {
			return fmt.Errorf("delete holdings for %s: %w", symbol, err)
		}

		_, err = s.db.ExecContext(ctx, `DELETE FROM dividend_income_records WHERE symbol = $1`, symbol)
		if err != nil {
			return fmt.Errorf("delete dividend records for %s: %w", symbol, err)
		}

		return nil
	}

	splits, err := s.listSplitsForSymbol(ctx, symbol)
	if err != nil {
		return fmt.Errorf("list splits for %s: %w", symbol, err)
	}

	splitMap := map[string][]SplitEvent{symbol: splits}
	states := ComputeHoldingStates(txs, splitMap)
	state := states[symbol]

	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin recalculate %s: %w", symbol, err)
	}
	defer dbTx.Rollback() //nolint:errcheck // rollback is a no-op after Commit; the Commit error is the one that matters

	oldTAAEnabled := true

	var oldPrice float64

	var oldAllocType, oldCurrency string

	_ = dbTx.QueryRowContext(ctx,
		`SELECT COALESCE(market_price, 0), COALESCE(taa_enabled, true),`+
			` COALESCE(allocation_type, 'SATELLITE'), COALESCE(currency, 'EUR')`+
			` FROM holdings WHERE symbol = $1 LIMIT 1`,
		symbol,
	).Scan(&oldPrice, &oldTAAEnabled, &oldAllocType, &oldCurrency)

	// If ComputeHoldingStates produced no allocation type (e.g. all SELLs), fall back to the
	// previously stored type so the DB NOT NULL constraint is satisfied.
	allocType := string(state.AllocationType)

	if allocType == "" {
		allocType = oldAllocType
	}

	// Preserve currency from state replay if available; otherwise keep DB value.
	currency := state.Currency

	if currency == "" {
		currency = oldCurrency
	}

	if currency == "" {
		currency = defaultCurrency
	}

	_, err = dbTx.ExecContext(ctx, `DELETE FROM holdings WHERE symbol = $1`, symbol)
	if err != nil {
		return fmt.Errorf("delete holdings for %s: %w", symbol, err)
	}

	// Always re-insert: active positions get real qty/PMC; closed positions get
	// qty=0 and pmc=0 but preserve taa_enabled, allocation_type, and currency for history.
	_, err = dbTx.ExecContext(ctx, `
		INSERT INTO holdings
			(symbol, quantity, market_price, pmc, allocation_type, taa_enabled, currency, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, symbol, state.Quantity, oldPrice, state.PMC,
		allocType, oldTAAEnabled, currency, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert holding for %s: %w", symbol, err)
	}

	err = dbTx.Commit()
	if err != nil {
		return fmt.Errorf("commit recalculate %s: %w", symbol, err)
	}

	return s.cleanupStaleDividendRecords(ctx, symbol, txs)
}
