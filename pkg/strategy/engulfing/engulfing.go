// Copyright (c) 2019 Simon Klinkert
// Copyright (c) 2026 Tommaso Gragnato
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// Package engulfing implements the engulfing candlestick pattern strategy.
package engulfing

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// Long: Buy if candle closes below the last 7 candles and is above SMA 200.
// Short: Short if candle closes above the last 7 candles and is below SMA 200.
// Source: https://www.youtube.com/watch?v=_9Bmxylp63Y

// Engulfing implements the engulfing candlestick pattern strategy.
type Engulfing struct {
	clog          *slog.Logger
	instrument    string
	sma           *sma.SMA
	ohlcPeriod    time.Duration
	openPositions []broker.Position
	openOrders    []broker.Order
}

const (
	targetInPercent      = 10.0
	stopLossInPercent    = 10.0
	smaCandles           = 200
	strategyLongEnabled  = true
	strategyShortEnabled = true
	minCandlesRequired   = 2
	fullPositionSize     = 1.00
)

// New creates a new Engulfing strategy instance.
func New(instrument string, candleDuration time.Duration) *Engulfing {
	clog := slog.With("INSTRUMENT", instrument, "CANDLE", candleDuration)

	return &Engulfing{
		clog:          clog,
		instrument:    instrument,
		sma:           sma.New(smaCandles),
		ohlcPeriod:    candleDuration,
		openPositions: []broker.Position{},
		openOrders:    []broker.Order{},
	}
}

// GetCandleDuration returns the candle duration for this strategy.
func (d *Engulfing) GetCandleDuration() time.Duration {
	return d.ohlcPeriod
}

// GetWarmUpCandleAmount returns the number of warm-up candles required.
func (d *Engulfing) GetWarmUpCandleAmount() uint {
	return smaCandles
}

// Name returns the strategy name.
func (d *Engulfing) Name() string {
	return strategy.NameEngulfing
}

// OnCandle processes a closed candle and returns orders and positions to manage.
//
func (d *Engulfing) OnCandle(
	closedCandles []*ohlc.OHLC,
) ([]broker.Order, []broker.Order, []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]
	defer d.feedIndicator(closedCandle)

	toOpen := make([]broker.Order, 0)
	toClosePositions := make([]broker.Position, 0)

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
func (d *Engulfing) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

// OnPosition updates the list of open positions.
func (d *Engulfing) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

// OnTick processes a tick — the engulfing strategy does not act on ticks.
//
func (d *Engulfing) OnTick(_ tick.Tick) ([]broker.Order, []broker.Order, []broker.Position) {
	return nil, nil, nil
}

// OnWarmUpCandle processes a warm-up candle for indicator initialization.
func (d *Engulfing) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.feedIndicator(closedCandle)
}

// Score returns a continuous conviction in [-1.0, +1.0] based on the strength of the engulfing pattern.
// A score of +1.0 indicates a strong bearish engulfing (large previous candle, small current candle body).
// A score of -1.0 indicates a strong bullish engulfing (small previous candle, large current candle body).
// A score of 0 indicates no engulfing pattern is present.
func (d *Engulfing) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) < minCandlesRequired {
		return 0
	}

	second := closedCandles[len(closedCandles)-2]
	first := closedCandles[len(closedCandles)-1]

	// Calculate the body sizes using float64.
	prevBody := math.Abs(second.Close - second.Open)
	currentBody := math.Abs(first.Close - first.Open)

	if prevBody == 0 {
		return 0
	}

	// Calculate the ratio of the current body size to the previous body size.
	ratio := currentBody / prevBody

	// Define thresholds for strong/weak engulfing.
	// A ratio significantly greater than 1.0 indicates a strong engulfing.
	minRatio := 0.5 // Weak engulfing
	maxRatio := 2.0 // Strong engulfing

	// Map the ratio to a continuous score between 0 and 1 (strength of engulfing).
	// If ratio is <= minRatio, score is 1.0 (strongest signal).
	// If ratio is >= maxRatio, score is 0 (weakest signal).
	var score float64

	switch {
	case ratio <= minRatio:
		score = 1.0
	case ratio >= maxRatio:
		score = 0.0
	default:
		// Linear interpolation between 0 and 1: (maxRatio - ratio) / (maxRatio - minRatio)
		numerator := maxRatio - ratio
		denominator := maxRatio - minRatio
		score = numerator / denominator
	}

	// Apply direction based on the type of engulfing.
	if d.isBearishEngulfingCandle(closedCandles) {
		// Bearish engulfing (counter-trend long): positive score.
		return score
	}

	if d.isBullishEngulfingCandle(closedCandles) {
		// Bullish engulfing (counter-trend short): negative score.
		return -score
	}

	return 0
}

// String returns a human-readable description of the strategy.
func (d *Engulfing) String() string {
	return fmt.Sprintf("%s: Long=%t, Short=%t Target=%.2f%% StopLoss=%.2f%% SMA%d", d.Name(),
		strategyLongEnabled, strategyShortEnabled, targetInPercent, stopLossInPercent, smaCandles)
}

func (d *Engulfing) feedIndicator(closedCandle *ohlc.OHLC) {
	d.sma.Insert(closedCandle)
}

// Rules:
// 1. close(-1) > open(-1)
// 2. close (0) < open(0)
// 3. open(0) > close(-1)
// 4. close(0) < open(-1).
func (d *Engulfing) isBearishEngulfingCandle(closedCandles []*ohlc.OHLC) bool {
	if len(closedCandles) < minCandlesRequired {
		return false
	}

	currentCandle := closedCandles[len(closedCandles)-1]
	previousCandle := closedCandles[len(closedCandles)-2]

	// Rule 1
	if !(previousCandle.Close > previousCandle.Open) {
		return false
	}

	// Rule 2
	if !(currentCandle.Close < currentCandle.Open) {
		return false
	}

	// Rule 3
	if !(currentCandle.Open > previousCandle.Close) {
		return false
	}

	// Rule 4
	if !(currentCandle.Close < previousCandle.Open) {
		return false
	}

	return true
}

// Rules:
// 1. close(0) > open(0)
// 2. close(-1) < open(-1)
// 3. open(-1) > close(0)
// 4. close(-1) < open(0).
func (d *Engulfing) isBullishEngulfingCandle(closedCandles []*ohlc.OHLC) bool {
	if len(closedCandles) < minCandlesRequired {
		return false
	}

	currentCandle := closedCandles[len(closedCandles)-1]
	previousCandle := closedCandles[len(closedCandles)-2]

	// Rule 1
	if !(currentCandle.Close > currentCandle.Open) {
		return false
	}

	// Rule 2
	if !(previousCandle.Close < previousCandle.Open) {
		return false
	}

	// Rule 3
	if !(previousCandle.Open > currentCandle.Close) {
		return false
	}

	// Rule 4
	if !(previousCandle.Close < currentCandle.Open) {
		return false
	}

	return true
}

func (d *Engulfing) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
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

//nolint:dupl // strategyLong and strategyShort are symmetric by design
func (d *Engulfing) strategyLong(closedCandles []*ohlc.OHLC) ([]broker.Order, []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]
	closePrice := closedCandle.Close
	lowPrice := closedCandle.Low

	if len(closedCandles) < minCandlesRequired {
		return nil, nil
	}

	previousCandle := closedCandles[len(closedCandles)-2]

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)

		return nil, nil
	}

	smaPrice := smaValue[sma.Value]

	for i := range d.openPositions {
		if d.openPositions[i].BuyDirection != broker.BuyDirectionLong {
			continue
		}

		if closedCandle.Close > previousCandle.Close {
			return nil, d.openPositions
		}
	}

	if len(d.openPositions) > 0 {
		return nil, nil
	}

	if closePrice < smaPrice && lowPrice < smaPrice {
		d.clog.Debug("close is below SMA", "close", closePrice, "sma", smaPrice)

		return nil, nil
	}

	if d.isBearishEngulfingCandle(closedCandles) {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong, fullPositionSize)

		return []broker.Order{toOpenNew}, []broker.Position{}
	}

	return nil, nil
}

//nolint:dupl // strategyShort and strategyLong are symmetric by design
func (d *Engulfing) strategyShort(closedCandles []*ohlc.OHLC) ([]broker.Order, []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]
	closePrice := closedCandle.Close
	lowPrice := closedCandle.Low

	if len(closedCandles) < minCandlesRequired {
		return nil, nil
	}

	previousCandle := closedCandles[len(closedCandles)-2]

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)

		return nil, nil
	}

	smaPrice := smaValue[sma.Value]

	for i := range d.openPositions {
		if d.openPositions[i].BuyDirection != broker.BuyDirectionShort {
			continue
		}

		if closedCandle.Close < previousCandle.Close {
			return nil, d.openPositions
		}
	}

	if len(d.openPositions) > 0 {
		return nil, nil
	}

	if closePrice > smaPrice && lowPrice > smaPrice {
		d.clog.Debug("close is above SMA", "close", closePrice, "sma", smaPrice)

		return nil, nil
	}

	if d.isBullishEngulfingCandle(closedCandles) {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort, fullPositionSize)

		return []broker.Order{toOpenNew}, []broker.Position{}
	}

	return nil, nil
}
