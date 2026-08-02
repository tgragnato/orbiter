package trader

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/tick"
)

func (tr *Trader) ensureSchema(ctx context.Context) error {
	if tr.db == nil {
		return nil
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS ticks (
			id BIGSERIAL PRIMARY KEY,
			datetime TIMESTAMPTZ NOT NULL,
			instrument TEXT NOT NULL,
			bid NUMERIC(13,6) NOT NULL,
			ask NUMERIC(13,6) NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ticks_instrument_datetime ON ticks (instrument, datetime)`,
		`CREATE TABLE IF NOT EXISTS ohlcs (
			id BIGSERIAL PRIMARY KEY,
			instrument TEXT NOT NULL,
			open NUMERIC(13,6) NOT NULL,
			high NUMERIC(13,6) NOT NULL,
			high_time TIMESTAMPTZ NOT NULL,
			low NUMERIC(13,6) NOT NULL,
			low_time TIMESTAMPTZ NOT NULL,
			close NUMERIC(13,6) NOT NULL,
			start TIMESTAMPTZ NOT NULL,
			end_time TIMESTAMPTZ NOT NULL,
			duration_ns BIGINT NOT NULL,
			gaps BOOLEAN NOT NULL DEFAULT FALSE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ohlcs_instrument_duration_start ON ohlcs (instrument, duration_ns, start)`,
		`CREATE TABLE IF NOT EXISTS performance_records (
			id BIGSERIAL PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			backtesting_id TEXT,
			strategy_name TEXT,
			strategy TEXT,
			instrument TEXT,
			candle_duration_ns BIGINT,
			target_in_pips DOUBLE PRECISION,
			stop_loss_in_pips DOUBLE PRECISION,
			performance_trigger DOUBLE PRECISION,
			total_performance_in_pips DOUBLE PRECISION,
			avg_performance_in_pips DOUBLE PRECISION,
			max_aggregate_drawdown_in_pips DOUBLE PRECISION,
			max_loss_in_pips DOUBLE PRECISION,
			max_loss_in_percent DOUBLE PRECISION,
			max_win_in_percent DOUBLE PRECISION,
			max_win_in_pips DOUBLE PRECISION,
			trades_win_ration_in_percent DOUBLE PRECISION,
			trades INTEGER,
			trades_win INTEGER,
			trades_loss INTEGER,
			trades_loss_long INTEGER,
			trades_loss_short INTEGER,
			trades_long INTEGER,
			trades_short INTEGER,
			max_consecutive_trades_loss BIGINT,
			max_concurrent_positions INTEGER,
			git_rev TEXT,
			duration TEXT,
			first_trade TIMESTAMPTZ,
			last_trade TIMESTAMPTZ,
			avg_trade_duration_in_seconds DOUBLE PRECISION,
			total_exposure_in_percent DOUBLE PRECISION,
			chart_html TEXT,
			backtesting_config_json TEXT,
			total_time_in_market_ns BIGINT,
			avg_time_in_market_ns BIGINT
		)`,
		`CREATE TABLE IF NOT EXISTS positions (
			id BIGSERIAL PRIMARY KEY,
			performance_record_id BIGINT REFERENCES performance_records(id) ON DELETE CASCADE,
			reference TEXT,
			instrument TEXT,
			allocation_type TEXT NOT NULL DEFAULT 'SATELLITE',
			buy_price NUMERIC(20,10),
			buy_time TIMESTAMPTZ,
			buy_direction SMALLINT,
			sell_price NUMERIC(20,10),
			sell_time TIMESTAMPTZ,
			target_price NUMERIC(20,10),
			stop_loss_price NUMERIC(20,10),
			size DOUBLE PRECISION,
			ohlc_age_on_buy_ns BIGINT,
			candle_buy_time TIMESTAMPTZ,
			candle_sell_time TIMESTAMPTZ,
			max_surge DOUBLE PRECISION,
			max_drawdown DOUBLE PRECISION,
			today_performance_in_percent NUMERIC(20,10),
			gap_to_sma NUMERIC(20,10)
		)`,
	}

	for _, q := range queries {
		if _, err := tr.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}

func (tr *Trader) insertTick(ctx context.Context, t tick.Tick) error {
	if tr.db == nil {
		return nil
	}
	_, err := tr.db.ExecContext(ctx,
		`INSERT INTO ticks (datetime, instrument, bid, ask) VALUES ($1, $2, $3, $4)`,
		t.Datetime,
		t.Instrument,
		t.Bid.String(),
		t.Ask.String(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (tr *Trader) loadWarmUpCandles(ctx context.Context, instrument string, duration time.Duration, limit int) ([]ohlc.OHLC, error) {
	if tr.db == nil {
		return nil, nil
	}

	rows, err := tr.db.QueryContext(ctx, `
		SELECT instrument, open, high, high_time, low, low_time, close, start, end_time, duration_ns, gaps
		FROM ohlcs
		WHERE instrument = $1 AND duration_ns = $2
		ORDER BY end_time DESC
		LIMIT $3`, instrument, int64(duration), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	candles := make([]ohlc.OHLC, 0, limit)
	for rows.Next() {
		candle, err := scanOHLC(rows)
		if err != nil {
			return nil, err
		}
		candles = append(candles, candle)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return candles, nil
}

func scanOHLC(row interface{ Scan(dest ...any) error }) (ohlc.OHLC, error) {
	var c ohlc.OHLC
	var durationNS int64
	var openStr, highStr, lowStr, closeStr string

	err := row.Scan(
		&c.Instrument,
		&openStr,
		&highStr,
		&c.HighTime,
		&lowStr,
		&c.LowTime,
		&closeStr,
		&c.Start,
		&c.End,
		&durationNS,
		&c.Gaps,
	)
	if err != nil {
		return ohlc.OHLC{}, err
	}

	openDec, err := decimal.NewFromString(openStr)
	if err != nil {
		return ohlc.OHLC{}, fmt.Errorf("invalid open decimal %q: %w", openStr, err)
	}
	highDec, err := decimal.NewFromString(highStr)
	if err != nil {
		return ohlc.OHLC{}, fmt.Errorf("invalid high decimal %q: %w", highStr, err)
	}
	lowDec, err := decimal.NewFromString(lowStr)
	if err != nil {
		return ohlc.OHLC{}, fmt.Errorf("invalid low decimal %q: %w", lowStr, err)
	}
	closeDec, err := decimal.NewFromString(closeStr)
	if err != nil {
		return ohlc.OHLC{}, fmt.Errorf("invalid close decimal %q: %w", closeStr, err)
	}

	c.Open = openDec
	c.High = highDec
	c.Low = lowDec
	c.Close = closeDec
	c.Duration = time.Duration(durationNS)
	return c, nil
}

var _ interface{ Scan(dest ...any) error } = (*sql.Row)(nil)
