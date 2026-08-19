// Package harami implements the bullish harami candlestick pattern strategy.
package harami

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// Buy if current closed candle is a bullish harami candle and market is still above SMA 200
// Source: https://www.youtube.com/watch?v=_9Bmxylp63Y

// Harami is a strategy based on the bullish harami candlestick pattern.
type Harami struct {
	clog          *slog.Logger
	instrument    string
	sma           *sma.SMA
	previousLows  *circularbuffer.CircularBuffer
	previousHighs *circularbuffer.CircularBuffer
	ohlcPeriod    time.Duration
	openPositions []broker.Position
}

const (
	targetInPercent      = 5.0
	stopLossInPercent    = 0.5
	smaCandles           = 200
	previousCandlesCount = 7
	defaultOrderSize     = 1.0
	minCandlesForScore   = 2
)

// New creates a new Harami strategy instance for the given instrument and candle duration.
func New(instrument string, candleDuration time.Duration) *Harami {
	clog := slog.With("INSTRUMENT", instrument, "CANDLE", candleDuration)

	return &Harami{
		clog:          clog,
		instrument:    instrument,
		sma:           sma.New(smaCandles),
		previousLows:  circularbuffer.New(previousCandlesCount, previousCandlesCount),
		previousHighs: circularbuffer.New(previousCandlesCount, previousCandlesCount),
		ohlcPeriod:    candleDuration,
		openPositions: nil,
	}
}

// GetCandleDuration returns the candle duration used by this strategy.
func (h *Harami) GetCandleDuration() time.Duration {
	return h.ohlcPeriod
}

// GetWarmUpCandleAmount returns the number of candles needed to warm up the strategy.
func (h *Harami) GetWarmUpCandleAmount() uint {
	return smaCandles
}

// Name returns the strategy name.
func (h *Harami) Name() string {
	return strategy.NameHarami
}

// OnCandle processes a closed candle and returns orders to open, close, and positions to close.
func (h *Harami) OnCandle( //nolint:nonamedreturns // named returns used to satisfy gocritic unnamedResult
	closedCandles []*ohlc.OHLC,
) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]

	closePrice := closedCandle.Close

	defer h.feedIndicator(closedCandle)

	smaValue, err := h.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)

		return nil, nil, nil
	}

	smaPrice := smaValue[sma.Value]

	if len(h.openPositions) > 0 {
		if closePrice < smaPrice {
			return nil, nil, h.openPositions
		}

		previousCandlesHigh, err := h.previousHighs.Max()
		if err != nil {
			return nil, nil, nil
		}

		if closedCandle.Close > previousCandlesHigh {
			return nil, nil, h.openPositions
		}

		return nil, nil, nil
	}

	latestCandle := closedCandles[len(closedCandles)-1]

	secondLatestCandle := closedCandles[len(closedCandles)-2]
	if h.isHaramiLong(secondLatestCandle, latestCandle) {
		toOpenNew := h.prepareOrder(closedCandle, broker.BuyDirectionLong, defaultOrderSize)

		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}

	h.clog.Debug("no harami long candle found", "candle", closedCandle)

	return nil, nil, nil
}

// OnOrder is called when orders are updated (no-op for this strategy).
func (h *Harami) OnOrder(_ []broker.Order) {}

// OnPosition is called when positions are updated (no-op for this strategy).
func (h *Harami) OnPosition(_, _ []broker.Position) {}

// OnTick is called on every tick (no-op for this strategy).
func (h *Harami) OnTick( //nolint:nonamedreturns // named returns used to satisfy gocritic unnamedResult
	_ tick.Tick,
) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	return nil, nil, nil
}

// OnWarmUpCandle feeds an OHLC candle into the strategy indicators during warm-up.
func (h *Harami) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	h.feedIndicator(closedCandle)
}

// Score returns a continuous conviction in [-1.0, +1.0] based on the strength of the Harami pattern.
// A score of +1.0 indicates a strong bullish Harami (large previous candle, small current candle body).
// A score of -1.0 indicates a strong bearish Harami (large previous candle, small current candle body).
// A score of 0 indicates no Harami pattern is present.
func (h *Harami) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) < minCandlesForScore {
		return 0
	}

	second := closedCandles[len(closedCandles)-2]
	first := closedCandles[len(closedCandles)-1]

	// Calculate the relative size of the current candle body compared to the previous candle body.
	prevBody := math.Abs(second.Close - second.Open)
	currentBody := math.Abs(first.Close - first.Open)

	if prevBody == 0 {
		return 0
	}

	// Calculate the ratio of the current body size to the previous body size.
	// A smaller ratio means a stronger Harami signal.
	ratio := currentBody / prevBody

	// Define thresholds using float64 for consistent comparison
	minRatio := 0.1
	maxRatio := 1.0

	// Linear interpolation between 0 and 1: (maxRatio - ratio) / (maxRatio - minRatio)
	numerator := maxRatio - ratio
	denominator := maxRatio - minRatio
	score := numerator / denominator

	// Apply direction based on the candle type
	if first.Close > first.Open {
		// Bullish Harami
		if score > 1.0 {
			return 1.0
		}

		return score
	} else if first.Close < first.Open {
		// Bearish Harami
		if score > 1.0 {
			return -1.0
		}

		return -score
	}

	return 0
}

// String returns a human-readable description of the strategy.
func (h *Harami) String() string {
	return fmt.Sprintf("%s: Target=%.2f StopLoss=%.2f", h.Name(), targetInPercent, stopLossInPercent)
}

func (h *Harami) feedIndicator(closedCandle *ohlc.OHLC) {
	high := closedCandle.High

	low := closedCandle.Low
	h.sma.Insert(closedCandle)
	h.previousLows.Insert(low)
	h.previousHighs.Insert(high)
}

func (h *Harami) isBearishCandle(candle *ohlc.OHLC) bool {
	return candle.Close < candle.Open
}

func (h *Harami) isBullishCandle(candle *ohlc.OHLC) bool {
	return candle.Close > candle.Open
}

func (h *Harami) isHaramiLong(firstCandle, secondCandle *ohlc.OHLC) bool {
	if h.isBearishCandle(firstCandle) && h.isBullishCandle(secondCandle) &&
		secondCandle.High < firstCandle.Open &&
		secondCandle.Low > firstCandle.Close {
		return true
	}

	return false
}

func (h *Harami) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
	var (
		targetPrice   = helper.CalcTargetPriceByPercentage(closedCandle.Close, targetInPercent, direction)
		stopLossPrice = helper.CalcStopLossPriceByPercentage(closedCandle.Close, stopLossInPercent, direction)
	)

	h.clog.Debug("Prepare new order",
		"Direction", direction.String(),
		"Time", closedCandle.End,
		"Close", closedCandle.Close,
		"Target", targetInPercent,
		"StopLoss", stopLossPrice,
	)

	return broker.NewMarketOrder(direction, size, h.instrument, targetPrice, stopLossPrice)
}
