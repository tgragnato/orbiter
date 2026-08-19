// Package rsi implements a trading strategy based on the Relative Strength Index indicator.
package rsi

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/helper"
	indicatorrsi "github.com/tgragnato/orbiter/pkg/indicator/rsi"
	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// RSI is a strategy that trades based on the Relative Strength Index indicator.
type RSI struct {
	clog           *slog.Logger
	instrument     string
	rsi            *indicatorrsi.RSI
	sma            *sma.SMA
	candleDuration time.Duration
	openPositions  []broker.Position
	openOrders     []broker.Order
}

const (
	upperThreshold     = 75
	lowerThreshold     = 25
	targetInPercent    = 0.2
	stopLossInPercent  = 0.6
	rsiSize            = 14
	smaCandles         = 200
	maxAgeOpenPosition = time.Hour * 2
	warmUpMultiplier   = 10
	fullOrderSize      = 1.00
)

// New creates a new RSI strategy instance for the given instrument and candle duration.
func New(instrument string, candleDuration time.Duration) *RSI {
	clog := slog.With("INSTRUMENT", instrument)

	return &RSI{
		clog:           clog,
		instrument:     instrument,
		rsi:            indicatorrsi.New(rsiSize),
		sma:            sma.New(smaCandles),
		candleDuration: candleDuration,
		openPositions:  nil,
		openOrders:     nil,
	}
}

// GetCandleDuration returns the candle duration used by this strategy.
func (d *RSI) GetCandleDuration() time.Duration {
	return d.candleDuration
}

// GetWarmUpCandleAmount returns the number of candles needed to warm up the indicators.
func (d *RSI) GetWarmUpCandleAmount() uint {
	return rsiSize * warmUpMultiplier
}

// Name returns the strategy name.
func (d *RSI) Name() string {
	return strategy.NameRSI
}

// OnCandle processes a new closed candle and returns orders and positions to manage.
//
//nolint:cyclop,nonamedreturns // multi-case trading logic; names clarify the two []broker.Order returns
func (d *RSI) OnCandle(
	closedCandles []*ohlc.OHLC,
) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]
	d.rsi.Insert(closedCandle)
	d.sma.Insert(closedCandle)

	for i := range d.openPositions {
		openPosition := d.openPositions[i]

		if openPosition.Age(closedCandle.End) > maxAgeOpenPosition &&
			openPosition.PerformanceAbsolute(closedCandle.Close, closedCandle.Close) > 0 {
			toClosePositions = append(toClosePositions, openPosition)

			continue
		}

		switch openPosition.BuyDirection {
		case broker.BuyDirectionShort:
			if d.isRSILongSignal() {
				toClosePositions = append(toClosePositions, openPosition)

				// counter position
				if d.isSMALongSignal(closedCandle.Close) {
					toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong)
					toOpen = append(toOpen, toOpenNew)
				}
			}
		case broker.BuyDirectionLong:
			if d.isRSIShortSignal() {
				toClosePositions = append(toClosePositions, openPosition)

				// counter position
				if d.isSMAShortSignal(closedCandle.Close) {
					toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort)
					toOpen = append(toOpen, toOpenNew)
				}
			}
		}
	}

	if len(d.openPositions) > 0 {
		return toOpen, toClose, toClosePositions
	}

	if d.isRSIShortSignal() && d.isSMAShortSignal(closedCandle.Close) {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort)

		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}

	if d.isRSILongSignal() && d.isSMALongSignal(closedCandle.Close) {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong)

		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}

	return toOpen, toClose, toClosePositions
}

// OnOrder updates the list of open orders.
func (d *RSI) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

// OnPosition updates the list of open positions.
func (d *RSI) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

// OnTick processes a tick event (not used by this strategy).
//
//nolint:nonamedreturns // names clarify the two identical []broker.Order return types
func (d *RSI) OnTick(_ tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	return nil, nil, nil
}

// OnWarmUpCandle feeds a warm-up candle into the indicators.
func (d *RSI) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.rsi.Insert(closedCandle)
	d.sma.Insert(closedCandle)
}

// Score returns a directional conviction in [-1.0, +1.0] based on the current
// RSI value. RSI <= lowerThreshold -> +1.0 (oversold/buy); RSI >= upperThreshold
// -> -1.0 (overbought/sell). SMA confirmation is not required here so the ML
// ensemble can learn to combine indicators on its own.
func (d *RSI) Score(_ []*ohlc.OHLC) float64 {
	rsiVal, err := d.getRSIValues()
	if err != nil {
		return 0
	}

	const mid = 50.0

	switch {
	case rsiVal <= lowerThreshold:
		return 1.0
	case rsiVal >= upperThreshold:
		return -1.0
	case rsiVal < mid:
		return (mid - rsiVal) / (mid - lowerThreshold)
	default:
		return -(rsiVal - mid) / (upperThreshold - mid)
	}
}

// String returns the strategy name as a string.
func (d *RSI) String() string {
	return d.Name()
}

func (d *RSI) getRSIValues() (float64, error) {
	rsiValueMap, err := d.rsi.Value()
	if err != nil {
		slog.Warn("Cannot get value from indicator", "error", err)

		return 0, fmt.Errorf("rsi value: %w", err)
	}

	rsiValue := rsiValueMap[indicatorrsi.Value]

	return rsiValue, nil
}

func (d *RSI) isRSILongSignal() bool {
	rsiValue, err := d.getRSIValues()

	return rsiValue <= lowerThreshold && err == nil
}

func (d *RSI) isRSIShortSignal() bool {
	rsiValue, err := d.getRSIValues()

	return rsiValue >= upperThreshold && err == nil
}

func (d *RSI) isSMALongSignal(closePrice float64) bool {
	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)

		return false
	}

	return closePrice > smaValue[sma.Value]
}

func (d *RSI) isSMAShortSignal(closePrice float64) bool {
	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)

		return false
	}

	return closePrice < smaValue[sma.Value]
}

func (d *RSI) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection) broker.Order {
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

	return broker.NewMarketOrder(direction, fullOrderSize, d.instrument, targetPrice, stopLossPrice)
}
