package portfolio

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

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
	return &PostgresStore{db: db}
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
		SELECT id, symbol, quantity, market_price, pmc, allocation_type, taa_enabled
		FROM holdings
		ORDER BY symbol, id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	holdings := make([]Holding, 0)
	for rows.Next() {
		var h Holding
		var allocation string
		if err := rows.Scan(&h.ID, &h.Symbol, &h.Quantity, &h.MarketPrice, &h.PMC, &allocation, &h.TAAEnabled); err != nil {
			return nil, err
		}
		h.AllocationType = parseAllocationType(allocation)
		holdings = append(holdings, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return holdings, nil
}

// ToggleAllocation swaps one holding allocation between CORE and SATELLITE.
func (s *PostgresStore) ToggleAllocation(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE holdings
		SET allocation_type = CASE allocation_type
			WHEN 'CORE' THEN 'SATELLITE'
			ELSE 'CORE'
		END,
		updated_at = NOW()
		WHERE id = $1
	`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("holding id %d not found", id)
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
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("holding symbol %q not found", symbol)
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
	for _, st := range states {
		total += st.RealizedPnL
	}
	return total, nil
}

func parseAllocationType(value string) AllocationType {
	upper := strings.ToUpper(strings.TrimSpace(value))
	if upper == string(AllocationCore) {
		return AllocationCore
	}
	return AllocationSatellite
}

// AddTransaction inserts a trade record and immediately recalculates only the
// affected symbol's holding quantity and PMC from its full transaction history.
// When WithPortfolioID has been set, it also appends a cash_flows row so TWR
// stays approximately correct within the current session without waiting for the
// next startup backfill.
func (s *PostgresStore) AddTransaction(ctx context.Context, tx Transaction) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO transactions
			(symbol, transaction_type, quantity, price, fee, allocation_type, executed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, tx.Symbol, string(tx.Type), tx.Quantity, tx.Price, tx.Fee,
		string(tx.AllocationType), tx.ExecutedAt)
	if err != nil {
		return fmt.Errorf("insert transaction: %w", err)
	}
	if err := s.recalculateSymbol(ctx, tx.Symbol); err != nil {
		return err
	}
	if s.portfolioID != "" {
		if err := s.recordLiveFlow(ctx, tx); err != nil {
			// Non-fatal: TWR will be corrected on next startup backfill.
			slog.Warn("portfolio: live cash-flow record failed", "symbol", tx.Symbol, "error", err)
		}
	}
	return nil
}

// recordLiveFlow inserts a single cash-flow row for a transaction executed in
// the current session. It uses tx.Price (not Yahoo AdjustedClose) because the
// adjusted price for today's candle is not yet available. The next startup
// backfill will replace all asset='AUTO' rows with adjusted-price equivalents,
// so this is only an intra-session approximation.
func (s *PostgresStore) recordLiveFlow(ctx context.Context, tx Transaction) error {
	var flowType string
	var amount float64
	day := tx.ExecutedAt.UTC().Truncate(24 * time.Hour)

	switch tx.Type {
	case TransactionBuy:
		flowType = "DEPOSIT"
		amount = tx.Quantity*tx.Price + tx.Fee
	case TransactionSell:
		flowType = "WITHDRAWAL"
		amount = tx.Quantity*tx.Price - tx.Fee
		if amount <= 0 {
			return nil
		}
	default:
		return nil
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO cash_flows (portfolio_id, flow_type, amount, asset, occurred_at)
		VALUES ($1, $2, $3, 'AUTO', $4)
	`, s.portfolioID, flowType, amount, day)
	return err
}

// UpdateTransaction overwrites a trade record identified by tx.ID and
// recalculates the affected holding(s). If the symbol changes, both the old
// and the new symbol's holding are recalculated.
func (s *PostgresStore) UpdateTransaction(ctx context.Context, tx Transaction) error {
	var oldSymbol string
	_ = s.db.QueryRowContext(ctx, `SELECT symbol FROM transactions WHERE id = $1`, tx.ID).Scan(&oldSymbol)

	_, err := s.db.ExecContext(ctx, `
		UPDATE transactions
		SET symbol = $1, transaction_type = $2, quantity = $3, price = $4,
		    fee = $5, allocation_type = $6, executed_at = $7
		WHERE id = $8
	`, tx.Symbol, string(tx.Type), tx.Quantity, tx.Price, tx.Fee,
		string(tx.AllocationType), tx.ExecutedAt, tx.ID)
	if err != nil {
		return fmt.Errorf("update transaction %d: %w", tx.ID, err)
	}
	if err := s.recalculateSymbol(ctx, tx.Symbol); err != nil {
		return err
	}
	if oldSymbol != "" && oldSymbol != tx.Symbol {
		return s.recalculateSymbol(ctx, oldSymbol)
	}
	return nil
}

// DeleteTransaction removes a trade record and recalculates the affected holding.
// If the symbol had only this one transaction, the holding is removed entirely.
func (s *PostgresStore) DeleteTransaction(ctx context.Context, id int64) error {
	var symbol string
	if err := s.db.QueryRowContext(ctx, `SELECT symbol FROM transactions WHERE id = $1`, id).Scan(&symbol); err != nil {
		return fmt.Errorf("find transaction %d: %w", id, err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM transactions WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete transaction %d: %w", id, err)
	}
	return s.recalculateSymbol(ctx, symbol)
}

// recalculateSymbol recomputes quantity and PMC for a single symbol by replaying
// its full transaction history. It preserves the existing market_price and
// taa_enabled flag; closed positions (zero net quantity) are kept with qty=0.
func (s *PostgresStore) recalculateSymbol(ctx context.Context, symbol string) error {
	txs, err := s.ListTransactions(ctx, symbol)
	if err != nil {
		return err
	}

	// No transactions remain — remove the holding and all dividend records entirely.
	if len(txs) == 0 {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM holdings WHERE symbol = $1`, symbol); err != nil {
			return err
		}
		_, err = s.db.ExecContext(ctx, `DELETE FROM dividend_income_records WHERE symbol = $1`, symbol)
		return err
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
	var oldAllocType string
	_ = dbTx.QueryRowContext(ctx,
		`SELECT COALESCE(market_price, 0), COALESCE(taa_enabled, true), COALESCE(allocation_type, 'SATELLITE') FROM holdings WHERE symbol = $1 LIMIT 1`,
		symbol,
	).Scan(&oldPrice, &oldTAAEnabled, &oldAllocType)

	// If ComputeHoldingStates produced no allocation type (e.g. all SELLs), fall back to the
	// previously stored type so the DB NOT NULL constraint is satisfied.
	allocType := string(state.AllocationType)
	if allocType == "" {
		allocType = oldAllocType
	}

	if _, err := dbTx.ExecContext(ctx, `DELETE FROM holdings WHERE symbol = $1`, symbol); err != nil {
		return fmt.Errorf("delete holdings for %s: %w", symbol, err)
	}

	// Always re-insert: active positions get real qty/PMC; closed positions get
	// qty=0 and pmc=0 but preserve taa_enabled and allocation_type for history.
	if _, err := dbTx.ExecContext(ctx, `
		INSERT INTO holdings
			(symbol, quantity, market_price, pmc, allocation_type, taa_enabled, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, symbol, state.Quantity, oldPrice, state.PMC,
		allocType, oldTAAEnabled, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("insert holding for %s: %w", symbol, err)
	}

	if err := dbTx.Commit(); err != nil {
		return err
	}
	return s.cleanupStaleDividendRecords(ctx, symbol, txs)
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
		if err := rows.Scan(&exDate); err != nil {
			return fmt.Errorf("scan ex_date for cleanup: %w", err)
		}
		if QuantityAtDate(txs, exDate) <= 0 {
			toDelete = append(toDelete, exDate)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, d := range toDelete {
		if _, err := s.db.ExecContext(ctx,
			`DELETE FROM dividend_income_records WHERE symbol = $1 AND ex_date = $2`,
			symbol, d); err != nil {
			return fmt.Errorf("delete stale dividend %s %s: %w", symbol, d.Format("2006-01-02"), err)
		}
	}
	return nil
}

// ListTransactions returns trade records ordered by execution time.
// Pass an empty symbol to list across all symbols.
func (s *PostgresStore) ListTransactions(ctx context.Context, symbol string) ([]Transaction, error) {
	query := `
		SELECT id, symbol, transaction_type, quantity, price, fee,
		       allocation_type, executed_at, created_at
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
		var t Transaction
		var txType, allocType string
		if err := rows.Scan(
			&t.ID, &t.Symbol, &txType, &t.Quantity, &t.Price, &t.Fee,
			&allocType, &t.ExecutedAt, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		t.Type = TransactionType(strings.ToUpper(txType))
		t.AllocationType = parseAllocationType(allocType)
		txs = append(txs, t)
	}
	return txs, rows.Err()
}

// RecalculateHoldings recomputes quantity and PMC for every symbol that has at
// least one transaction, using the Italian weighted-average cost method. Existing
// market_price and taa_enabled values are preserved; closed positions keep qty=0.
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
		_ = dbTx.QueryRowContext(ctx,
			`SELECT COALESCE(market_price, 0), COALESCE(taa_enabled, true) FROM holdings WHERE symbol = $1 LIMIT 1`,
			symbol,
		).Scan(&oldPrice, &oldTAAEnabled)

		if _, err := dbTx.ExecContext(ctx,
			`DELETE FROM holdings WHERE symbol = $1`, symbol,
		); err != nil {
			return fmt.Errorf("delete holdings for %s: %w", symbol, err)
		}

		if _, err := dbTx.ExecContext(ctx, `
			INSERT INTO holdings
				(symbol, quantity, market_price, pmc, allocation_type, taa_enabled, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, symbol, state.Quantity, oldPrice, state.PMC,
			string(state.AllocationType), oldTAAEnabled, time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("insert holding for %s: %w", symbol, err)
		}
	}

	return dbTx.Commit()
}

// UpdateMarketPrice stores a fresh market quote for the named holding.
func (s *PostgresStore) UpdateMarketPrice(ctx context.Context, symbol string, price float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE holdings SET market_price = $1, updated_at = NOW() WHERE symbol = $2`,
		price, symbol,
	)
	return err
}

// FirstTransactionDate returns the execution date of the earliest transaction
// for the given symbol, used to determine the start of the candle history needed
// for dividend backfill.
func (s *PostgresStore) FirstTransactionDate(ctx context.Context, symbol string) (time.Time, error) {
	var t time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT MIN(executed_at) FROM transactions WHERE symbol = $1`, symbol,
	).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("first transaction date for %s: %w", symbol, err)
	}
	return t.UTC(), nil
}

// UpsertDividendIncomes inserts or updates dividend income records.
// Records are keyed on (symbol, ex_date); existing rows are overwritten so
// that a re-run with updated quantities always reflects the current position.
func (s *PostgresStore) UpsertDividendIncomes(ctx context.Context, records []DividendRecord) error {
	for _, r := range records {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO dividend_income_records
				(symbol, ex_date, quantity, cash_dividend_per_share, income_amount)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (symbol, ex_date) DO UPDATE SET
				quantity               = EXCLUDED.quantity,
				cash_dividend_per_share = EXCLUDED.cash_dividend_per_share,
				income_amount          = EXCLUDED.income_amount
		`, r.Symbol, r.ExDate, r.Quantity, r.CashDividendPerShare, r.IncomeAmount)
		if err != nil {
			return fmt.Errorf("upsert dividend income %s %s: %w", r.Symbol, r.ExDate.Format("2006-01-02"), err)
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
		SELECT symbol, ex_date, quantity, cash_dividend_per_share, income_amount
		FROM dividend_income_records
		ORDER BY ex_date DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list dividend income: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []DividendRecord
	for rows.Next() {
		var r DividendRecord
		if err := rows.Scan(&r.Symbol, &r.ExDate, &r.Quantity, &r.CashDividendPerShare, &r.IncomeAmount); err != nil {
			return nil, fmt.Errorf("scan dividend income: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
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
		if err := rows.Scan(&sym); err != nil {
			return nil, fmt.Errorf("scan symbol: %w", err)
		}
		symbols = append(symbols, sym)
	}
	return symbols, rows.Err()
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
		if err := rows.Scan(&sym); err != nil {
			return nil, fmt.Errorf("scan symbol: %w", err)
		}
		symbols = append(symbols, sym)
	}
	return symbols, rows.Err()
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
		var ev SplitEvent
		if err := rows.Scan(&ev.Date, &ev.Factor); err != nil {
			return nil, fmt.Errorf("scan split event: %w", err)
		}
		ev.Date = ev.Date.UTC()
		splits = append(splits, ev)
	}
	return splits, rows.Err()
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
		var ev SplitEvent
		if err := rows.Scan(&sym, &ev.Date, &ev.Factor); err != nil {
			return nil, fmt.Errorf("scan split row: %w", err)
		}
		ev.Date = ev.Date.UTC()
		out[sym] = append(out[sym], ev)
	}
	return out, rows.Err()
}

// UpsertSplit persists a split event for the given symbol. It is idempotent:
// re-inserting the same (symbol, split_date) with the same factor is a no-op.
func (s *PostgresStore) UpsertSplit(ctx context.Context, symbol string, splitDate time.Time, factor float64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO stock_splits (symbol, split_date, factor)
		VALUES ($1, $2, $3)
		ON CONFLICT (symbol, split_date) DO UPDATE SET factor = EXCLUDED.factor
	`, symbol, splitDate.UTC().Truncate(24*time.Hour), factor)
	if err != nil {
		return fmt.Errorf("upsert split %s %s: %w", symbol, splitDate.Format("2006-01-02"), err)
	}
	return nil
}
