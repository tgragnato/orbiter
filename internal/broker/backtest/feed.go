package backtest

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/piquette/finance-go/chart"
	"github.com/piquette/finance-go/datetime"
	"github.com/shopspring/decimal"
	"github.com/sklinkert/at/pkg/ohlc"
	"github.com/sklinkert/at/pkg/tick"
	"github.com/sklinkert/igmarkets"
)

type QuotesSource int

const (
	QuotesSourcePostgres     QuotesSource = 1
	QuotesSourceYahooFinance QuotesSource = 2
	QuotesSourceIGMarkets    QuotesSource = 3
	QuotesSourceCoinbase     QuotesSource = 4
)

func (b *Backtest) retrieveCandlesFromIGMarkets(receiver chan ohlc.OHLC) {
	defer close(receiver)

	var ctx = context.Background()

	if err := b.brokerIGMarkets.Login(ctx); err != nil {
		panic(fmt.Sprintf("login failed: %v", err))
	}

	priceResponse, err := b.brokerIGMarkets.GetPriceHistory(ctx, b.instrument, igmarkets.ResolutionHour, 100, b.periodFrom, b.periodTo)
	if err != nil {
		panic(fmt.Sprintf("failed to fetch price history for %q from IG Markets: %v", b.instrument, err))
	}
	slog.Info(fmt.Sprintf("prices fetched: %d", len(priceResponse.Prices)))

	for _, price := range priceResponse.Prices {
		open := bidAskToTick(b.instrument, price.SnapshotTimeUTCParsed, price.OpenPrice.Bid, price.OpenPrice.Ask)
		high := bidAskToTick(b.instrument, price.SnapshotTimeUTCParsed, price.HighPrice.Bid, price.HighPrice.Ask)
		low := bidAskToTick(b.instrument, price.SnapshotTimeUTCParsed, price.LowPrice.Bid, price.LowPrice.Ask)
		close := bidAskToTick(b.instrument, price.SnapshotTimeUTCParsed, price.ClosePrice.Bid, price.ClosePrice.Ask)

		var candle = ohlc.OHLC{
			Instrument: b.instrument,
			Open:       open.Price(),
			High:       high.Price(),
			Low:        low.Price(),
			Close:      close.Price(),
			Start:      price.SnapshotTimeUTCParsed,
			End:        price.SnapshotTimeUTCParsed.Add(b.priceDBCandleDuration),
		}
		candle.ForceClose()
		slog.Info(fmt.Sprintf("Candle: %+v", candle))
		receiver <- candle
	}
}

func bidAskToTick(instrument string, datetime time.Time, bid, ask float64) tick.Tick {
	return tick.New(instrument, datetime, decimal.NewFromFloat(bid), decimal.NewFromFloat(ask))
}

func (b *Backtest) retrieveCandlesFromYahooFinance(receiver chan ohlc.OHLC) {
	defer close(receiver)

	params := &chart.Params{
		Symbol:   b.instrument,
		Interval: datetime.OneDay,
		Start:    datetime.New(&b.periodFrom),
		End:      datetime.New(&b.periodTo),
	}

	slog.Info(fmt.Sprintf("Fetching quotes from Yahoo Finance for %q with period %s - %s",
		b.instrument, b.periodFrom, b.periodTo))
	iter := chart.Get(params)

	for iter.Next() {
		openTime := time.Unix(int64(iter.Bar().Timestamp), 0)
		bar := iter.Bar()

		candle := ohlc.OHLC{
			Instrument: b.instrument,
			Open:       bar.Open,
			High:       bar.High,
			Low:        bar.Low,
			Close:      bar.Close,
			Start:      openTime,
			End:        openTime.Add(b.priceDBCandleDuration),
		}
		candle.ForceClose()
		slog.Info(fmt.Sprintf("Candle: %+v", candle))

		receiver <- candle
	}
	if err := iter.Err(); err != nil {
		panic(fmt.Sprintf("getting quotes from yahoo failed: %v", err))
	}
}

func (b *Backtest) retrieveCandlesFromPostgres(receiver chan ohlc.OHLC) {
	defer close(receiver)

	ctx := context.Background()
	db, err := sql.Open("pgx", b.priceDBDSN)
	if err != nil {
		panic(fmt.Sprintf("failed to connect postgres %q: %v", b.priceDBDSN, err))
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		panic(fmt.Sprintf("failed to ping postgres %q: %v", b.priceDBDSN, err))
	}

	const pageSize = 80000
	var offset int
	for {
		rows, err := db.QueryContext(ctx, `
			SELECT instrument, open, high, high_time, low, low_time, close, start, end_time, duration_ns, gaps
			FROM ohlcs
			WHERE duration_ns = $1 AND start BETWEEN $2 AND $3
			ORDER BY start
			LIMIT $4 OFFSET $5`,
			int64(b.priceDBCandleDuration), b.periodFrom, b.periodTo, pageSize, offset)
		if err != nil {
			slog.Error("fetching candles failed", "error", err)
			return
		}

		var candles []ohlc.OHLC
		for rows.Next() {
			candle, err := scanOHLCRow(rows)
			if err != nil {
				rows.Close()
				slog.Error("scanning candle failed", "error", err)
				return
			}
			candles = append(candles, candle)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			slog.Error("iterating candles failed", "error", err)
			return
		}

		if len(candles) == 0 {
			slog.Info("No more candles fetched")
			return
		}
		for _, candle := range candles {
			receiver <- candle
		}
		offset += pageSize
	}
}

func (b *Backtest) ListenToPriceFeed(traderChan chan tick.Tick) {
	var c = make(chan ohlc.OHLC)

	switch b.quotesSource {
	case QuotesSourcePostgres:
		go b.retrieveCandlesFromPostgres(c)
	case QuotesSourceYahooFinance:
		go b.retrieveCandlesFromYahooFinance(c)
	case QuotesSourceIGMarkets:
		go b.retrieveCandlesFromIGMarkets(c)
	case QuotesSourceCoinbase:
		go b.retrieveCandlesFromCoinbase(c)
	default:
		panic(fmt.Sprintf("Unknown quotes source: %d", b.quotesSource))
	}

	for candle := range c {
		for _, currentTick := range candle.ToTicks() {
			b.paperwallet.SetCurrenctPrice(currentTick)
			traderChan <- currentTick
		}
	}
	b.paperwallet.CloseAllOpenPositions()
	b.writeCSV()
	b.paperwallet.PrintSummary()
}

func scanOHLCRow(row interface{ Scan(dest ...any) error }) (ohlc.OHLC, error) {
	var candle ohlc.OHLC
	var openStr, highStr, lowStr, closeStr string
	var durationNS int64

	err := row.Scan(
		&candle.Instrument,
		&openStr,
		&highStr,
		&candle.HighTime,
		&lowStr,
		&candle.LowTime,
		&closeStr,
		&candle.Start,
		&candle.End,
		&durationNS,
		&candle.Gaps,
	)
	if err != nil {
		return ohlc.OHLC{}, err
	}

	if candle.Open, err = decimal.NewFromString(openStr); err != nil {
		return ohlc.OHLC{}, err
	}
	if candle.High, err = decimal.NewFromString(highStr); err != nil {
		return ohlc.OHLC{}, err
	}
	if candle.Low, err = decimal.NewFromString(lowStr); err != nil {
		return ohlc.OHLC{}, err
	}
	if candle.Close, err = decimal.NewFromString(closeStr); err != nil {
		return ohlc.OHLC{}, err
	}
	candle.Duration = time.Duration(durationNS)

	return candle, nil
}
