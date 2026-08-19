package portfolio

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/tgragnato/orbiter/internal/portfolio/data"
)

// GetCachedCandles returns persisted EOD candles for symbol in [from, until].
// Implements data.CandleStorer.
func (s *PostgresStore) GetCachedCandles(
	ctx context.Context, symbol string, from, until time.Time,
) ([]data.Candle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT symbol, candle_date, open, high, low, close_price, adj_close,
		       volume, cash_dividend, split_factor, currency
		FROM eod_candles
		WHERE symbol = $1 AND candle_date >= $2 AND candle_date <= $3
		ORDER BY candle_date ASC`,
		symbol, from.UTC(), until.UTC())
	if err != nil {
		return nil, fmt.Errorf("query eod_candles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var candles []data.Candle

	for rows.Next() {
		var candle data.Candle

		var candleTime time.Time

		err = rows.Scan(
			&candle.Ticker, &candleTime,
			&candle.Open, &candle.High, &candle.Low, &candle.Close, &candle.AdjustedClose,
			&candle.Volume, &candle.CashDividend, &candle.SplitFactor, &candle.Currency,
		)
		if err != nil {
			return nil, fmt.Errorf("scan eod_candle row: %w", err)
		}

		candle.Time = candleTime.UTC()
		candles = append(candles, candle)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("iterate eod_candles: %w", err)
	}

	return candles, nil
}

// UpsertCandles stores candles in a single transaction using a prepared
// statement, updating existing rows on (symbol, candle_date) conflict.
// Implements data.CandleStorer.
func (s *PostgresStore) UpsertCandles(ctx context.Context, candles []data.Candle) error {
	if len(candles) == 0 {
		return nil
	}

	dbTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upsert candles transaction: %w", err)
	}

	stmt, err := dbTx.PrepareContext(ctx, `
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
		_ = dbTx.Rollback()

		return fmt.Errorf("prepare upsert candles statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, candle := range candles {
		_, err = stmt.ExecContext(ctx,
			candle.Ticker,
			candle.Time.UTC().Format("2006-01-02"),
			candle.Open, candle.High, candle.Low, candle.Close, candle.AdjustedClose,
			candle.Volume, candle.CashDividend, candle.SplitFactor, candle.Currency,
		)
		if err != nil {
			_ = dbTx.Rollback()

			return fmt.Errorf("upsert candle %s/%s: %w", candle.Ticker, candle.Time.Format("2006-01-02"), err)
		}
	}

	err = dbTx.Commit()
	if err != nil {
		return fmt.Errorf("commit upsert candles transaction: %w", err)
	}

	return nil
}

// LatestCandleDate returns the most recent cached candle date for symbol,
// or zero time if no candles are cached for that symbol.
// Implements data.CandleStorer.
func (s *PostgresStore) LatestCandleDate(ctx context.Context, symbol string) (time.Time, error) {
	var latestTime sql.NullTime

	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(candle_date) FROM eod_candles WHERE symbol = $1`, symbol).Scan(&latestTime)
	if err != nil {
		return time.Time{}, fmt.Errorf("query latest candle date for %s: %w", symbol, err)
	}

	if !latestTime.Valid {
		return time.Time{}, nil // no cached data for this symbol
	}

	return latestTime.Time.UTC(), nil
}
