// Package stochrsi implements a trading strategy based on the Stochastic RSI indicator.
package stochrsi

import (
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/indicator/stochrsi"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// RSI is a strategy that uses the Stochastic RSI indicator to generate trading signals.
type RSI struct {
	clog          *slog.Logger
	instrument    string
	rsi           *stochrsi.StochRSI
	openPositions []broker.Position
	openOrders    []broker.Order
}

const (
	ohlcPeriod        = time.Minute * 60
	upperThreshold    = 90
	lowerThreshold    = 10
	middleThreshold   = (upperThreshold + lowerThreshold) / 2
	targetInPercent   = 4.0
	stopLossInPercent = 0.5

	stochK       = 5
	stochD       = 2
	rsiPeriod    = 14
	warmUpAmount = 99
	defaultSize  = 1.00
	avgDivisor   = 2.0
)

// New creates a new RSI strategy for the given instrument.
func New(instrument string) *RSI {
	clog := slog.With("INSTRUMENT", instrument)

	return &RSI{
		clog:          clog,
		instrument:    instrument,
		rsi:           stochrsi.New(stochK, stochD, rsiPeriod),
		openPositions: nil,
		openOrders:    nil,
	}
}

// GetCandleDuration returns the duration of the OHLC candle used by this strategy.
func (d *RSI) GetCandleDuration() time.Duration {
	return ohlcPeriod
}

// GetWarmUpCandleAmount returns the number of warm-up candles required.
func (d *RSI) GetWarmUpCandleAmount() uint {
	return warmUpAmount
}

// Name returns the strategy name.
func (d *RSI) Name() string {
	return strategy.NameStochRSI
}

// OnCandle processes new closed candles and returns orders to open, close, and positions to close.
//
func (d *RSI) OnCandle(
	closedCandles []*ohlc.OHLC,
) ([]broker.Order, []broker.Order, []broker.Position) {
	if len(closedCandles) == 0 {
		return nil, nil, nil
	}

	closedCandle := closedCandles[len(closedCandles)-1]
	d.rsi.Insert(closedCandle)

	if len(d.openPositions) > 0 {
		return nil, nil, nil
	}

	rsiValueMap, err := d.rsi.Value()
	if err != nil {
		slog.Debug("Cannot get value from indicator", "error", err)

		return nil, nil, nil
	}

	kValue := rsiValueMap[stochrsi.ValueK]
	dValue := rsiValueMap[stochrsi.ValueD]

	if dValue == 0 {
		return nil, nil, nil
	}

	if kValue > upperThreshold && dValue > upperThreshold {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort, defaultSize)

		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}

	if kValue < lowerThreshold && dValue < lowerThreshold {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong, defaultSize)

		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}

	return nil, nil, nil
}

// OnOrder updates the list of open orders.
func (d *RSI) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

// OnPosition updates the list of open positions.
func (d *RSI) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

// OnTick is called on each market tick and returns no orders for this strategy.
//
func (d *RSI) OnTick(_ tick.Tick) ([]broker.Order, []broker.Order, []broker.Position) {
	return nil, nil, nil
}

// OnWarmUpCandle feeds a historical candle into the RSI indicator during warm-up.
func (d *RSI) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.rsi.Insert(closedCandle)
}

// Score returns a directional conviction in [-1.0, +1.0] from the average of
// the StochRSI K and D lines. Above upperThreshold → -1.0; below lowerThreshold
// → +1.0. Interpolated linearly in between.
func (d *RSI) Score(_ []*ohlc.OHLC) float64 {
	rsiValueMap, err := d.rsi.Value()
	if err != nil {
		return 0
	}

	k := rsiValueMap[stochrsi.ValueK]
	dv := rsiValueMap[stochrsi.ValueD]
	avg := (k + dv) / avgDivisor

	switch {
	case avg >= upperThreshold:
		return -1.0
	case avg <= lowerThreshold:
		return 1.0
	case avg < middleThreshold:
		return (middleThreshold - avg) / (middleThreshold - lowerThreshold)
	default:
		return -(avg - middleThreshold) / (upperThreshold - middleThreshold)
	}
}

// String returns the strategy name as a string.
func (d *RSI) String() string {
	return d.Name()
}

func (d *RSI) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
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
