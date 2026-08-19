// Package doji implements the DOJI candlestick pattern strategy.
package doji

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

var (
	// ErrUnknownDirection is returned when the direction is not recognized.
	ErrUnknownDirection = errors.New("unknown direction")
	// ErrNoPreviousCandle is returned when the previous candle is nil.
	ErrNoPreviousCandle = errors.New("previousCandle is nil")
)

// Doji detects a DOJI candle (tiny body ≤ 0.025% change) and enters on the next
// tick that breaks above the previous high or below the previous low by 2 pips.
type Doji struct {
	clog           *slog.Logger
	instrument     string
	previousCandle *ohlc.OHLC
	openPositions  []broker.Position
	openOrders     []broker.Order
}

const (
	ohlcPeriod       = time.Minute * 60
	decZero          = 0
	targetInPercent  = 0.045
	dec2Pip          = 0.0002
	dojiThreshold    = 0.025
	percentDivisor   = 100.0
	defaultOrderSize = 1.00
)

// New creates a new Doji strategy instance for the given instrument.
func New(instrument string) *Doji {
	clog := slog.With("INSTRUMENT", instrument)

	return &Doji{
		clog:           clog,
		instrument:     instrument,
		previousCandle: nil,
		openPositions:  nil,
		openOrders:     nil,
	}
}

// GetCandleDuration returns the candle duration for this strategy.
func (d *Doji) GetCandleDuration() time.Duration {
	return ohlcPeriod
}

// GetWarmUpCandleAmount returns the number of warm-up candles required.
func (d *Doji) GetWarmUpCandleAmount() uint {
	return 1
}

// Name returns the strategy name.
func (d *Doji) Name() string {
	return strategy.NameDOJI
}

// OnCandle records the latest closed candle for tick-level breakout detection.
//
func (d *Doji) OnCandle(closedCandles []*ohlc.OHLC) ([]broker.Order, []broker.Order, []broker.Position) {
	d.previousCandle = closedCandles[len(closedCandles)-1]

	return nil, nil, nil
}

// OnOrder updates the list of open orders.
func (d *Doji) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

// OnPosition updates the list of open positions.
func (d *Doji) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

// OnTick fires entry orders when the current tick breaks a preceding DOJI candle's range.
//
func (d *Doji) OnTick(currentTick tick.Tick) ([]broker.Order, []broker.Order, []broker.Position) {
	if len(d.openPositions) > 0 {
		return nil, nil, nil
	}

	if !isDOJI(d.previousCandle) {
		return nil, nil, nil
	}

	if currentTick.Bid > d.previousCandle.High+dec2Pip {
		order, err := d.createOrder(currentTick, targetInPercent, broker.BuyDirectionLong, defaultOrderSize)
		if err == nil {
			return []broker.Order{order}, nil, nil
		}

		return nil, nil, nil
	}

	if currentTick.Ask < d.previousCandle.Low-dec2Pip {
		order, err := d.createOrder(currentTick, targetInPercent, broker.BuyDirectionShort, defaultOrderSize)
		if err == nil {
			return []broker.Order{order}, nil, nil
		}
	}

	return nil, nil, nil
}

// OnWarmUpCandle records the warm-up candle.
func (d *Doji) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.previousCandle = closedCandle
}

// Score returns a continuous conviction in [-1.0, +1.0] based on the proximity of the current
// candle to the DOJI candle's range. If the current candle is a DOJI, the score is 0.
// If the current candle breaks the range, the score is 1.0 (buy) or -1.0 (sell).
func (d *Doji) Score(_ []*ohlc.OHLC) float64 {
	if isDOJI(d.previousCandle) {
		return 0
	}

	// Check if the current tick/candle breaks the range.
	// Since Score is called on closed candles, we check the last closed candle.
	lastCandle := d.previousCandle
	if lastCandle == nil {
		return 0
	}

	// Calculate the distance from the DOJI range.
	// If the close is above the high, it's positive (buy).
	// If the close is below the low, it's negative (sell).

	// We use a sensitivity factor to determine how quickly the score reaches 1.0 or -1.0.
	// A value of 100 pips (0.0010) is used as a normalization factor for the "strength" of the breakout.
	sensitivity := 0.0010

	score := 0.0

	if lastCandle.Close > lastCandle.High {
		// Distance above high.
		dist := lastCandle.Close - lastCandle.High
		// Normalize: score = dist / sensitivity, clamped to 1.0.
		score = dist / sensitivity
		if score > 1.0 {
			score = 1.0
		}
	} else if lastCandle.Close < lastCandle.Low {
		// Distance below low.
		dist := lastCandle.Low - lastCandle.Close
		// Normalize: score = - (dist / sensitivity), clamped to -1.0.
		score -= dist / sensitivity
		if score < -1.0 {
			score = -1.0
		}
	}

	return score
}

// String returns an empty string.
func (d *Doji) String() string {
	return ""
}

func (d *Doji) calcStopLossPrice(direction broker.BuyDirection) (float64, error) {
	if d.previousCandle == nil {
		return decZero, ErrNoPreviousCandle
	}

	switch direction {
	case broker.BuyDirectionLong:
		return d.previousCandle.Low, nil
	case broker.BuyDirectionShort:
		return d.previousCandle.High, nil
	default:
		return decZero, ErrUnknownDirection
	}
}

func (d *Doji) calcTargetPrice(
	direction broker.BuyDirection, currentTick tick.Tick, perfMarginInPercentage float64,
) (float64, error) {
	switch direction {
	case broker.BuyDirectionLong:
		currentPrice := currentTick.Ask
		percentFrom := currentPrice * perfMarginInPercentage / percentDivisor

		return currentPrice + percentFrom, nil
	case broker.BuyDirectionShort:
		currentPrice := currentTick.Bid
		percentFrom := currentPrice * perfMarginInPercentage / percentDivisor

		return currentPrice - percentFrom, nil
	default:
		return decZero, ErrUnknownDirection
	}
}

func (d *Doji) createOrder(
	currentTick tick.Tick,
	perfMargin float64,
	direction broker.BuyDirection,
	size float64,
) (broker.Order, error) {
	targetPrice, err := d.calcTargetPrice(direction, currentTick, perfMargin)
	if err != nil {
		return broker.Order{}, fmt.Errorf("calcTargetPrice() failed: %w", err)
	}

	stopLossPrice, err := d.calcStopLossPrice(direction)
	if err != nil {
		return broker.Order{}, fmt.Errorf("calcStopLossPrice() failed: %w", err)
	}

	d.clog.Debug("Creating new order",
		"Direction", direction.String(),
		"Time", currentTick.Datetime,
		"Bid", currentTick.Bid,
		"Ask", currentTick.Ask,
		"PerfMargin", perfMargin,
		"TargetPrice", targetPrice,
		"StopLossPrice", stopLossPrice,
	)

	return broker.NewMarketOrder(direction, size, d.instrument, targetPrice, stopLossPrice), nil
}

func isDOJI(candle *ohlc.OHLC) bool {
	if candle == nil || !candle.Closed() {
		return false
	}

	perfPercentage := math.Abs(candle.PerformanceInPercentage())

	return perfPercentage <= dojiThreshold
}
