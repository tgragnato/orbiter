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
	"github.com/sklinkert/at/internal/broker/coinbase"
	"github.com/sklinkert/at/internal/paperwallet"
	"github.com/sklinkert/at/internal/strategy/rsiadx"
	"github.com/sklinkert/at/internal/trader"
)

// Example setup for Coinbase broker

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

func mustConnectDB(ctx context.Context) *sql.DB {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/at?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fatal("failed to connect database", err)
	}
	if err := db.PingContext(ctx); err != nil {
		fatal(fmt.Sprintf("failed to ping database with DSN %q", dsn), err)
	}
	return db
}

func main() {
	ctx := context.Background()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	instrument := "BTC-USD"
	candleDuration := time.Minute * 1
	strategyBackend := rsiadx.New(instrument, candleDuration)
	wallet := paperwallet.New()
	brokerBackend := coinbase.New(instrument, wallet)

	db := mustConnectDB(ctx)
	defer db.Close()

	tr := trader.New(ctx, instrument, "", db,
		trader.WithBroker(brokerBackend),
		trader.WithPersistCandleData(true),
		trader.WithStrategy(strategyBackend),
		trader.WithFeedStoredCandles(strategyBackend),
	)
	if err := tr.Start(); err != nil {
		fatal("failed to start trader", err)
	}
	tr.Summary()
}
