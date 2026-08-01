package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"github.com/sklinkert/at/internal/broker/backtest"
	"github.com/sklinkert/at/internal/broker/coinbase"
	"github.com/sklinkert/at/internal/broker/ig"
	"github.com/sklinkert/at/internal/paperwallet"
	"github.com/sklinkert/at/internal/strategy/rsiadx"
	"github.com/sklinkert/at/internal/trader"
	chart "github.com/sklinkert/at/pkg/chart"
	"github.com/sklinkert/at/pkg/chart/amcharts"
	"github.com/sklinkert/at/pkg/histdatacom"
	"github.com/sklinkert/at/pkg/ohlc"
	"github.com/sklinkert/at/pkg/tick"
)

func RunBacktesting(ctx context.Context, gitRev string, args []string) error {
	conf, err := parseBacktestingFlags(args)
	if err != nil {
		return err
	}
	setupLogging(conf.debug)

	db, err := openPostgres(ctx, conf.dbDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	candleDuration, err := time.ParseDuration(conf.candleDuration)
	if err != nil {
		return fmt.Errorf("cannot parse candle duration: %w", err)
	}

	strategyBackend, err := buildStrategy(conf.strategyName, conf.instrument, candleDuration)
	if err != nil {
		return err
	}

	options := []backtest.Option{backtest.WithCandlePeriod(candleDuration)}
	if len(conf.importHistDataCSVFiles) == 0 {
		options = append(options, backtest.WithPriceDBDSN(conf.priceDBDSN, time.Minute))
	} else {
		options = append(options, backtest.WithTickDataFiles(conf.importHistDataCSVFiles))
	}
	switch conf.priceSource {
	case "COINBASE":
		options = append(options, backtest.WithQuotesSource(backtest.QuotesSourceCoinbase))
	default:
		options = append(options, backtest.WithQuotesSource(backtest.QuotesSourcePostgres))
	}

	initialBalance := decimal.NewFromFloat(1000)
	tradingFeePercent := decimal.NewFromFloat(0.01)
	wallet := paperwallet.New(
		paperwallet.WithInitialBalance(initialBalance),
		paperwallet.WithTradingFeePercent(tradingFeePercent),
	)

	periodFrom := time.Date(conf.yearFrom, time.Month(conf.monthFrom), 1, 0, 0, 0, 0, time.UTC)
	periodTo := time.Date(conf.yearTo, time.Month(conf.monthTo), 31, 23, 23, 59, 0, time.UTC)
	brokerBackend := backtest.New(conf.instrument, periodFrom, periodTo, wallet, options...)
	var graph chart.Chart = amcharts.NewChart(conf.instrument)

	tr := trader.New(ctx, conf.instrument, gitRev, db,
		trader.WithBroker(brokerBackend),
		trader.WithStrategy(strategyBackend),
		trader.WithCandleSubscription(graph),
		trader.WithPositionSubscription(graph),
	)
	if err := tr.Start(); err != nil {
		return err
	}

	chartHTML, err := graph.RenderChartToHTML()
	if err != nil {
		return fmt.Errorf("unable to render chart as HTML: %w", err)
	}
	if err := tr.SavePerformanceRecord(chartHTML); err != nil {
		return err
	}
	tr.Summary()

	if err := graph.Start(); err != nil {
		return err
	}
	return nil
}

func RunIG(ctx context.Context, gitRev string, args []string) error {
	if gitRev == "" {
		return errors.New("GitRev not set")
	}

	conf, err := parseIGFlags(args)
	if err != nil {
		return err
	}
	setupLogging(conf.debug)

	candleDuration, err := time.ParseDuration(conf.candleDuration)
	if err != nil {
		return fmt.Errorf("cannot parse candle duration: %w", err)
	}

	strategyBackend, err := buildStrategy(conf.strategyName, conf.instrument, candleDuration)
	if err != nil {
		return err
	}

	brokerBackend, err := ig.New(conf.instrument, conf.igAPIURL, conf.igAPIKey, conf.igAccountID,
		conf.igIdentifier, conf.igPassword)
	if err != nil {
		return fmt.Errorf("ig.New() failed: %w", err)
	}

	db, err := openPostgres(ctx, conf.dbDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	tr := trader.New(ctx, conf.instrument, gitRev, db,
		trader.WithBroker(brokerBackend),
		trader.WithPersistCandleData(true),
		trader.WithStrategy(strategyBackend),
		trader.WithFeedStoredCandles(strategyBackend),
		trader.WithCurrencyCode(conf.currencyCode),
	)
	if err := tr.Start(); err != nil {
		return err
	}
	tr.Summary()
	return nil
}

func RunCoinbase(ctx context.Context, gitRev string, args []string) error {
	conf, err := parseCoinbaseFlags(args)
	if err != nil {
		return err
	}
	setupLogging(conf.debug)

	candleDuration, err := time.ParseDuration(conf.candleDuration)
	if err != nil {
		return fmt.Errorf("cannot parse candle duration: %w", err)
	}

	strategyBackend := rsiadx.New(conf.instrument, candleDuration)
	wallet := paperwallet.New()
	brokerBackend := coinbase.New(conf.instrument, wallet)

	db, err := openPostgres(ctx, conf.dbDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	tr := trader.New(ctx, conf.instrument, gitRev, db,
		trader.WithBroker(brokerBackend),
		trader.WithPersistCandleData(true),
		trader.WithStrategy(strategyBackend),
		trader.WithFeedStoredCandles(strategyBackend),
	)
	if err := tr.Start(); err != nil {
		return err
	}
	tr.Summary()
	return nil
}

func RunImportHistdata(ctx context.Context, args []string) error {
	conf, csvFiles, instrument, err := parseHistdataFlags(args)
	if err != nil {
		return err
	}
	setupLogging(conf.debug)

	tickChan := make(chan tick.Tick)
	db, err := openPostgres(ctx, conf.dbDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := ensureSchema(ctx, db); err != nil {
		return fmt.Errorf("schema initialization failed: %w", err)
	}

	go histdatacom.ImportFromCSV(instrument, splitCSV(csvFiles), tickChan)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}

	var currentTime time.Time
	var imported uint
	var candle *ohlc.OHLC
	const candleDuration = time.Minute
	for currentTick := range tickChan {
		if candle != nil {
			if !candle.NewPrice(currentTick.Price(), currentTick.Datetime) {
				if err := insertOHLC(ctx, tx, candle); err != nil {
					return err
				}
				candle = nil
			}
		}
		if candle == nil {
			candle = ohlc.New(instrument, currentTick.Datetime, candleDuration, true)
			candle.NewPrice(currentTick.Price(), currentTick.Datetime)
		}
		if imported > 0 && imported%1000 == 0 {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("cannot commit transaction: %w", err)
			}
			tx, err = db.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("cannot begin transaction: %w", err)
			}
		}
		if currentTime.Day() != currentTick.Datetime.Day() {
			slog.Info(fmt.Sprintf("Importing day %s", currentTick.Datetime))
		}
		currentTime = currentTick.Datetime
		imported++
	}
	if candle != nil {
		if err := insertOHLC(ctx, tx, candle); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit final transaction: %w", err)
	}
	slog.Info(fmt.Sprintf("%d ticks imported", imported))
	return nil
}
