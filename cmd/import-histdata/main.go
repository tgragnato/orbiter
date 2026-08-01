package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lfritz/env"
	"github.com/sklinkert/at/pkg/histdatacom"
	"github.com/sklinkert/at/pkg/ohlc"
	"github.com/sklinkert/at/pkg/tick"
)

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

var conf struct {
	dbHost     string
	dbUser     string
	dbPassword string
	dbName     string
	dbPort     int
}

func main() {
	ctx := context.Background()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	var c = make(chan tick.Tick)
	var e = env.New()
	var csvFiles []string
	var instrument string
	e.List("IMPORT_HISTDATA_CSV_FILES", &csvFiles, ",", "Import CSV files from histdata.com")
	e.String("INSTRUMENT", &instrument, "Instrument name e.g. EURUSD")
	e.OptionalString("DB_HOST", &conf.dbHost, "", "DB host")
	e.OptionalString("DB_USER", &conf.dbUser, "guest", "DB user")
	e.OptionalString("DB_PASSWORD", &conf.dbPassword, "guest", "DB password")
	e.OptionalString("DB_NAME", &conf.dbName, "guest", "DB name")
	e.OptionalInt("DB_PORT", &conf.dbPort, 25060, "DB port")
	if err := e.Load(); err != nil {
		fatal("env loading failed", err)
	}

	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=require",
			conf.dbHost, conf.dbUser, conf.dbPassword, conf.dbName, conf.dbPort)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fatal("failed to connect database", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		fatal("db ping failed", err)
	}

	if err := ensureSchema(ctx, db); err != nil {
		fatal("schema initialization failed", err)
	}

	go histdatacom.ImportFromCSV(instrument, csvFiles, c)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fatal("cannot begin transaction", err)
	}

	var currentTime time.Time
	var imported uint
	var candle *ohlc.OHLC
	const candleDuration = time.Minute * 1
	for currentTick := range c {
		if candle != nil {
			isOpen := candle.NewPrice(currentTick.Price(), currentTick.Datetime)
			if !isOpen {
				if err := insertOHLC(ctx, tx, candle); err != nil {
					fatal("Cannot store candle", err)
				}
				candle = nil // force new candle opening
			}
		}
		if candle == nil {
			candle = ohlc.New(instrument, currentTick.Datetime, candleDuration, true)
			candle.NewPrice(currentTick.Price(), currentTick.Datetime)
		}
		//if err := tx.Create(&currentTick).Error; err != nil {
		//	log.WithError(err).Warn("db.Create() failed: %v", currentTick)
		//	continue
		//}
		if imported%1000 == 0 {
			if err := tx.Commit(); err != nil {
				fatal("cannot commit transaction", err)
			}
			tx, err = db.BeginTx(ctx, nil)
			if err != nil {
				fatal("cannot begin transaction", err)
			}
		}

		if currentTime.Day() != currentTick.Datetime.Day() {
			slog.Info(fmt.Sprintf("Importing day %s", currentTick.Datetime))
		}
		currentTime = currentTick.Datetime
		imported++
	}
	if err := tx.Commit(); err != nil {
		fatal("cannot commit final transaction", err)
	}
	slog.Info(fmt.Sprintf("%d ticks imported", imported))
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
	for _, q := range queries {
		if _, err := db.ExecContext(ctx, q); err != nil {
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
	if err != nil {
		return err
	}
	return nil
}
