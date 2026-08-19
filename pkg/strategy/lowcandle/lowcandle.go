package lowcandle

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// Long: Buy if candle closes below the last 7 candles and is above SMA 200
// Short: Short if candle closes above the last 7 candles and is below SMA 200
// Source: https://www.youtube.com/watch?v=_9Bmxylp63Y

// LowCandle implements a strategy based on breakouts below/above recent candle ranges.
type LowCandle struct {
	clog          *slog.Logger
	instrument    string
	sma           *sma.SMA
	previousLows  *circularbuffer.CircularBuffer
	previousHighs *circularbuffer.CircularBuffer
	ohlcPeriod    time.Duration
	openPositions []broker.Position
	openOrders    []broker.Order
}

const (
	targetInPercent      = 4.0
	stopLossInPercent    = 0.5
	smaCandles           = 200
	strategyLongEnabled  = true
	strategyShortEnabled = true
	lookbackCandles      = 7
	defaultOrderSize     = 1.00
)

// New creates a new LowCandle strategy instance for the given instrument and candle duration.
func New(instrument string, candleDuration time.Duration) *LowCandle {
	clog := slog.With("INSTRUMENT", instrument, "CANDLE", candleDuration)

	return &LowCandle{
		clog:          clog,
		instrument:    instrument,
		sma:           sma.New(smaCandles),
		previousLows:  circularbuffer.New(lookbackCandles, lookbackCandles),
		previousHighs: circularbuffer.New(lookbackCandles, lookbackCandles),
		ohlcPeriod:    candleDuration,
		openPositions: nil,
		openOrders:    nil,
	}
}

// GetCandleDuration returns the candle duration used by this strategy.
func (d *LowCandle) GetCandleDuration() time.Duration {
	return d.ohlcPeriod
}

// GetWarmUpCandleAmount returns the number of warm-up candles required.
func (d *LowCandle) GetWarmUpCandleAmount() uint {
	return smaCandles
}

// Name returns the strategy name.
func (d *LowCandle) Name() string {
	return strategy.NameLowCandle
}

// OnCandle processes new closed candles and returns orders and positions to open/close.
//
func (d *LowCandle) OnCandle(
	closedCandles []*ohlc.OHLC,
) ([]broker.Order, []broker.Order, []broker.Position) {
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
func (d *LowCandle) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

// OnPosition updates the list of open positions.
func (d *LowCandle) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

// OnTick processes a new tick and returns orders to open/close.
//
func (d *LowCandle) OnTick(_ tick.Tick) ([]broker.Order, []broker.Order, []broker.Position) {
	return nil, nil, nil
}

// OnWarmUpCandle processes a historical candle used to warm up indicators.
func (d *LowCandle) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.feedIndicator(closedCandle)
}

// Score returns a continuous conviction in [-1.0, +1.0] based on the current candle's
// position relative to the historical range (previous lows/highs).
// A score of +1.0 indicates a strong buy breakout (close significantly below previous lows).
// A score of -1.0 indicates a strong sell breakout (close significantly above previous highs).
// A score of 0 indicates the close is within the historical range.
func (d *LowCandle) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) == 0 {
		return 0
	}

	closePrice := closedCandles[len(closedCandles)-1].Close

	prevLowFloat, errL := d.previousLows.Min()
	prevHighFloat, errH := d.previousHighs.Max()

	if errL != nil || errH != nil {
		return 0
	}

	// Convert float64 to float64 for consistent arithmetic
	prevLow := prevLowFloat
	prevHigh := prevHighFloat

	// We need a normalization factor. Let's use the total historical range.
	totalRange := prevHigh - prevLow

	if totalRange == 0 {
		return 0
	}

	// Normalize the distance.
	// If close < prevLow, score = (prevLow - close) / totalRange (positive)
	if closePrice < prevLow {
		score := (prevLow - closePrice) / totalRange

		return score
	}

	// If close > prevHigh, score = (close - prevHigh) / totalRange (negative)
	if closePrice > prevHigh {
		score := (closePrice - prevHigh) / totalRange

		return -score
	}

	return 0
}

// String returns a human-readable description of the strategy configuration.
func (d *LowCandle) String() string {
	return fmt.Sprintf("%s: Long=%t, Short=%t Target=%.2f%% StopLoss=%.2f%% SMA%d", d.Name(),
		strategyLongEnabled, strategyShortEnabled, targetInPercent, stopLossInPercent, smaCandles)
}

func (d *LowCandle) feedIndicator(closedCandle *ohlc.OHLC) {
	var high = closedCandle.High

	var low = closedCandle.Low

	d.sma.Insert(closedCandle)
	d.previousLows.Insert(low)
	d.previousHighs.Insert(high)
}

func (d *LowCandle) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
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

//nolint:cyclop // complexity driven by multiple early-return guard clauses; extraction would obscure intent
func (d *LowCandle) strategyLong(closedCandles []*ohlc.OHLC) ([]broker.Order, []broker.Position) {
	var closedCandle = closedCandles[len(closedCandles)-1]

	var closePrice = closedCandle.Close

	var lowPrice = closedCandle.Low

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)

		return nil, nil
	}

	smaPrice := smaValue[sma.Value]

	for posIdx := range d.openPositions {
		if d.openPositions[posIdx].BuyDirection != broker.BuyDirectionLong {
			continue
		}

		previousCandlesHigh, err := d.previousHighs.Max()
		if err != nil {
			return nil, nil
		}

		if closedCandle.Close > previousCandlesHigh {
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

	previousCandlesLow, err := d.previousLows.Min()
	if err != nil {
		d.clog.Warn("no previous low", "error", err)

		return nil, nil
	}

	if closePrice < previousCandlesLow {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong, defaultOrderSize)

		return []broker.Order{toOpenNew}, []broker.Position{}
	}

	d.clog.Debug("long: closePrice >= previousCandlesLow", "close", closePrice, "previousLow", previousCandlesLow)

	return nil, nil
}

//nolint:cyclop // complexity driven by multiple early-return guard clauses; extraction would obscure intent
func (d *LowCandle) strategyShort(closedCandles []*ohlc.OHLC) ([]broker.Order, []broker.Position) {
	var closedCandle = closedCandles[len(closedCandles)-1]

	var closePrice = closedCandle.Close

	var highPrice = closedCandle.High

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)

		return nil, nil
	}

	smaPrice := smaValue[sma.Value]

	for posIdx := range d.openPositions {
		if d.openPositions[posIdx].BuyDirection != broker.BuyDirectionShort {
			continue
		}

		previousCandlesLow, err := d.previousLows.Min()
		if err != nil {
			return nil, nil
		}

		if closedCandle.Close < previousCandlesLow {
			return nil, []broker.Position{d.openPositions[posIdx]}
		}
	}

	if len(d.openPositions) > 0 {
		return nil, nil
	}

	if closePrice > smaPrice && highPrice > smaPrice {
		d.clog.Debug("close is above SMA", "close", closePrice, "sma", smaPrice)

		return nil, nil
	}

	previousCandlesHigh, err := d.previousHighs.Max()
	if err != nil {
		d.clog.Warn("no previous high", "error", err)

		return nil, nil
	}

	if closePrice > previousCandlesHigh {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort, defaultOrderSize)

		return []broker.Order{toOpenNew}, []broker.Position{}
	}

	d.clog.Debug("short: closePrice <= previousCandlesHigh", "close", closePrice, "previousHigh", previousCandlesHigh)

	return nil, nil
}
