package portfolio

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio/data"
)

// GetCachedCandles returns persisted EOD candles for symbol in [from, to].
// Implements data.CandleStorer.
func (s *PostgresStore) GetCachedCandles(ctx context.Context, symbol string, from, to time.Time) ([]data.Candle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT symbol, candle_date, open, high, low, close_price, adj_close,
		       volume, cash_dividend, split_factor, currency
		FROM eod_candles
		WHERE symbol = $1 AND candle_date >= $2 AND candle_date <= $3
		ORDER BY candle_date ASC`,
		symbol, from.UTC(), to.UTC())
	if err != nil {
		return nil, fmt.Errorf("query eod_candles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var candles []data.Candle
	for rows.Next() {
		var c data.Candle
		var t time.Time
		if err := rows.Scan(
			&c.Ticker, &t,
			&c.Open, &c.High, &c.Low, &c.Close, &c.AdjustedClose,
			&c.Volume, &c.CashDividend, &c.SplitFactor, &c.Currency,
		); err != nil {
			return nil, fmt.Errorf("scan eod_candle row: %w", err)
		}
		c.Time = t.UTC()
		candles = append(candles, c)
	}
	return candles, rows.Err()
}

// UpsertCandles stores candles in a single transaction using a prepared
// statement, updating existing rows on (symbol, candle_date) conflict.
// Implements data.CandleStorer.
func (s *PostgresStore) UpsertCandles(ctx context.Context, candles []data.Candle) error {
	if len(candles) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert candles transaction: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO eod_candles
		    (symbol, candle_date, open, high, low, close_price, adj_close,
		     volume, cash_dividend, split_factor, currency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (symbol, candle_date) DO UPDATE
		    SET open          = EXCLUDED.open,
		        high          = EXCLUDED.high,
		        low           = EXCLUDED.low,
		        close_price   = EXCLUDED.close_price,
		        adj_close     = EXCLUDED.adj_close,
		        volume        = EXCLUDED.volume,
		        cash_dividend = EXCLUDED.cash_dividend,
		        split_factor  = EXCLUDED.split_factor`)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("prepare upsert candles statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, c := range candles {
		if _, err := stmt.ExecContext(ctx,
			c.Ticker,
			c.Time.UTC().Format("2006-01-02"),
			c.Open, c.High, c.Low, c.Close, c.AdjustedClose,
			c.Volume, c.CashDividend, c.SplitFactor, c.Currency,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("upsert candle %s/%s: %w", c.Ticker, c.Time.Format("2006-01-02"), err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upsert candles transaction: %w", err)
	}
	return nil
}

// LatestCandleDate returns the most recent cached candle date for symbol,
// or zero time if no candles are cached for that symbol.
// Implements data.CandleStorer.
func (s *PostgresStore) LatestCandleDate(ctx context.Context, symbol string) (time.Time, error) {
	var t sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(candle_date) FROM eod_candles WHERE symbol = $1`, symbol).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("query latest candle date for %s: %w", symbol, err)
	}
	if !t.Valid {
		return time.Time{}, nil // no cached data for this symbol
	}
	return t.Time.UTC(), nil
}
