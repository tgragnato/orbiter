// Package sma10 implements a trading strategy based on SMA-10 and SMA-200 crossover.
package sma10

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// Long: Buy if candle closes below the last 7 candles and is above SMA 200
// Short: Short if candle closes above the last 7 candles and is below SMA 200
// Source: https://www.youtube.com/watch?v=_9Bmxylp63Y

// SMA is the strategy based on SMA-10 and SMA-200 crossover.
type SMA struct {
	clog          *slog.Logger
	instrument    string
	sma           *sma.SMA
	sma10         *sma.SMA
	ohlcPeriod    time.Duration
	openPositions []broker.Position
	openOrders    []broker.Order
}

const (
	targetInPercent      = 2.0
	stopLossInPercent    = 0.5
	smaCandles           = 200
	sma10Candles         = 10
	orderSize            = 1.00
	strategyLongEnabled  = true
	strategyShortEnabled = true
)

// New creates a new SMA strategy instance.
func New(instrument string, candleDuration time.Duration) *SMA {
	clog := slog.With("INSTRUMENT", instrument, "CANDLE", candleDuration)

	return &SMA{
		clog:          clog,
		instrument:    instrument,
		sma:           sma.New(smaCandles),
		sma10:         sma.New(sma10Candles),
		ohlcPeriod:    candleDuration,
		openPositions: nil,
		openOrders:    nil,
	}
}

// GetCandleDuration returns the candle duration for this strategy.
func (d *SMA) GetCandleDuration() time.Duration {
	return d.ohlcPeriod
}

// GetWarmUpCandleAmount returns the number of warm-up candles required.
func (d *SMA) GetWarmUpCandleAmount() uint {
	return smaCandles
}

// Name returns the strategy name.
func (d *SMA) Name() string {
	return strategy.NameSMA10
}

// OnCandle processes a new closed candle and returns orders to open/close.
func (d *SMA) OnCandle(closedCandles []*ohlc.OHLC) (
	[]broker.Order, []broker.Order, []broker.Position,
) {
	closedCandle := closedCandles[len(closedCandles)-1]
	defer d.feedIndicator(closedCandle)

	var toOpen []broker.Order

	var toClosePositions []broker.Position

	if strategyLongEnabled {
		toOpenLong, toCloseLong := d.strategyLong(closedCandles)
		toOpen = append(toOpen, toOpenLong...)
		toClosePositions = append(toClosePositions, toCloseLong...)
	}

	if strategyShortEnabled {
		toOpenShort, toCloseShort := d.strategyShort(closedCandles)
		toOpen = append(toOpen, toOpenShort...)
		toClosePositions = append(toClosePositions, toCloseShort...)
	}

	return toOpen, nil, toClosePositions
}

// OnOrder updates the list of open orders.
func (d *SMA) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

// OnPosition updates the list of open positions.
func (d *SMA) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

// OnTick processes a tick event; this strategy does not act on ticks.
//
func (d *SMA) OnTick(_ tick.Tick) ([]broker.Order, []broker.Order, []broker.Position) {
	return nil, nil, nil
}

// OnWarmUpCandle processes a warm-up candle to seed the indicators.
func (d *SMA) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.feedIndicator(closedCandle)
}

// Score returns a directional conviction in [-1.0, +1.0].
// Close above SMA-200 and below SMA-10 → positive (buy setup).
// Close below SMA-200 and above SMA-10 → negative (sell setup).
//
//nolint:cyclop // inherently complex scoring function with symmetric long/short logic
func (d *SMA) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) == 0 {
		return 0
	}

	smaValue, err := d.sma.Value()
	if err != nil {
		return 0
	}

	sma200 := smaValue[sma.Value]

	sma10val, err := d.sma10.Value()
	if err != nil {
		return 0
	}

	sma10 := sma10val[sma.Value]

	if sma200 == 0 || sma10 == 0 {
		return 0
	}

	closePrice := closedCandles[len(closedCandles)-1].Close

	// Long setup: above SMA-200 trend filter, price dipped below fast SMA-10.
	if closePrice > sma200 && closePrice < sma10 {
		if sma10-sma200 == 0 {
			return 0
		}

		conviction := (sma10 - closePrice) / (sma10 - sma200)
		if conviction > 1 {
			conviction = 1
		}

		return conviction
	}

	// Short setup: below SMA-200 trend filter, price risen above fast SMA-10.
	if closePrice < sma200 && closePrice > sma10 {
		if sma200-sma10 == 0 {
			return 0
		}

		conviction := (closePrice - sma10) / (sma200 - sma10)
		if conviction > 1 {
			conviction = 1
		}

		return -conviction
	}

	return 0
}

// String returns a human-readable description of the strategy.
func (d *SMA) String() string {
	return fmt.Sprintf("%s: Long=%t, Short=%t Target=%.2f%% StopLoss=%.2f%% SMA%d", d.Name(),
		strategyLongEnabled, strategyShortEnabled, targetInPercent, stopLossInPercent, smaCandles)
}

func (d *SMA) feedIndicator(closedCandle *ohlc.OHLC) {
	d.sma.Insert(closedCandle)
	d.sma10.Insert(closedCandle)
}

func (d *SMA) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
	var (
		targetPrice   = helper.CalcTargetPriceByPercentage(closedCandle.Close, targetInPercent, direction)
		stopLossPrice = helper.CalcStopLossPriceByPercentage(closedCandle.Close, stopLossInPercent, direction)
	)

	d.clog.Debug("Prepare new order",
		"Direction", direction.String(),
		"Time", closedCandle.End,
		"Close", closedCandle.Close,
		"Target", targetInPercent,
		"StopLoss", stopLossPrice,
	)

	return broker.NewMarketOrder(direction, size, d.instrument, targetPrice, stopLossPrice)
}

//nolint:dupl // strategyLong and strategyShort are intentionally symmetric
func (d *SMA) strategyLong(closedCandles []*ohlc.OHLC) ([]broker.Order, []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]

	closePrice := closedCandle.Close

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)

		return nil, nil
	}

	sma200Price := smaValue[sma.Value]

	sma10Val, err := d.sma10.Value()
	if err != nil {
		return nil, nil
	}

	sma10Price := sma10Val[sma.Value]

	for idx := range d.openPositions {
		if d.openPositions[idx].BuyDirection != broker.BuyDirectionLong {
			continue
		}

		if closedCandle.Close > sma10Price {
			return nil, d.openPositions
		}
	}

	if len(d.openPositions) > 0 {
		return nil, nil
	}

	if closePrice < sma200Price {
		d.clog.Debug("close is below SMA200", "close", closePrice, "sma200", sma200Price)

		return nil, nil
	}

	if closePrice < sma10Price {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong, orderSize)

		return []broker.Order{toOpenNew}, []broker.Position{}
	}

	d.clog.Debug("long: closePrice >= SMA10", "close", closePrice, "sma10", sma10Price)

	return nil, nil
}

//nolint:dupl // strategyLong and strategyShort are intentionally symmetric
func (d *SMA) strategyShort(closedCandles []*ohlc.OHLC) ([]broker.Order, []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]

	closePrice := closedCandle.Close

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)

		return nil, nil
	}

	sma200Price := smaValue[sma.Value]

	sma10Val, err := d.sma10.Value()
	if err != nil {
		return nil, nil
	}

	sma10Price := sma10Val[sma.Value]

	for idx := range d.openPositions {
		if d.openPositions[idx].BuyDirection != broker.BuyDirectionShort {
			continue
		}

		if closedCandle.Close < sma10Price {
			return nil, d.openPositions
		}
	}

	if len(d.openPositions) > 0 {
		return nil, nil
	}

	if closePrice > sma200Price {
		d.clog.Debug("close is above SMA200", "close", closePrice, "sma200", sma200Price)

		return nil, nil
	}

	if closePrice > sma10Price {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort, orderSize)

		return []broker.Order{toOpenNew}, []broker.Position{}
	}

	d.clog.Debug("short: closePrice <= SMA10", "close", closePrice, "sma10", sma10Price)

	return nil, nil
}
