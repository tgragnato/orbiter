package app

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/sklinkert/at/internal/strategy"
	heikinashi "github.com/sklinkert/at/internal/strategy/HeikinAshi"
	"github.com/sklinkert/at/internal/strategy/doji"
	"github.com/sklinkert/at/internal/strategy/engulfing"
	"github.com/sklinkert/at/internal/strategy/harami"
	"github.com/sklinkert/at/internal/strategy/lowcandle"
	"github.com/sklinkert/at/internal/strategy/rsi"
	"github.com/sklinkert/at/internal/strategy/rsiadx"
	"github.com/sklinkert/at/internal/strategy/scalper"
	"github.com/sklinkert/at/internal/strategy/sma10"
	"github.com/sklinkert/at/internal/strategy/stochrsi"
	"github.com/sklinkert/at/pkg/ohlc"
)

func setupLogging(debug bool) {
	lvl := new(slog.LevelVar)
	if debug {
		lvl.Set(slog.LevelDebug)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
}

func openPostgres(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database with DSN %q: %w", dsn, err)
	}
	return db, nil
}

func buildStrategy(name, instrument string, candleDuration time.Duration) (strategy.Strategy, error) {
	switch name {
	case strategy.NameDOJI:
		return doji.New(instrument), nil
	case strategy.NameHeikinAshi:
		return heikinashi.New(instrument), nil
	case strategy.NameScalper:
		return scalper.New(instrument), nil
	case strategy.NameStochRSI:
		return stochrsi.New(instrument), nil
	case strategy.NameLowCandle:
		return lowcandle.New(instrument, candleDuration), nil
	case strategy.NameHarami:
		return harami.New(instrument, candleDuration), nil
	case strategy.NameSMA10:
		return sma10.New(instrument, candleDuration), nil
	case strategy.NameEngulfing:
		return engulfing.New(instrument, candleDuration), nil
	case strategy.NameRSI:
		return rsi.New(instrument, candleDuration), nil
	case strategy.NameRSIADX:
		return rsiadx.New(instrument, candleDuration), nil
	default:
		return nil, fmt.Errorf("unsupported strategy %q", name)
	}
}

func ensureSchema(ctx context.Context, db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS ticks (
			id BIGSERIAL PRIMARY KEY,
			datetime TIMESTAMPTZ NOT NULL,
			instrument TEXT NOT NULL,
			bid NUMERIC(13,6) NOT NULL,
			ask NUMERIC(13,6) NOT NULL
		)`,
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
	}
	for _, query := range queries {
		if _, err := db.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return nil
}

func insertOHLC(ctx context.Context, tx *sql.Tx, candle *ohlc.OHLC) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ohlcs (
			instrument, open, high, high_time, low, low_time, close, start, end_time, duration_ns, gaps
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`,
		candle.Instrument,
		candle.Open.String(),
		candle.High.String(),
		candle.HighTime,
		candle.Low.String(),
		candle.LowTime,
		candle.Close.String(),
		candle.Start,
		candle.End,
		int64(candle.Duration),
		candle.Gaps,
	)
	return err
}
