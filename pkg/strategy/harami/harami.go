package harami

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

// Buy if current closed candle is a bullish harami candle and market is still above SMA 200
// Source: https://www.youtube.com/watch?v=_9Bmxylp63Y

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
	targetInPercent   = 5.0
	stopLossInPercent = 0.5
	smaCandles        = 200
)

func New(instrument string, candleDuration time.Duration) *Harami {
	clog := slog.With("INSTRUMENT", instrument, "CANDLE", candleDuration)

	return &Harami{
		clog:          clog,
		instrument:    instrument,
		sma:           sma.New(smaCandles),
		previousLows:  circularbuffer.New(7, 7),
		previousHighs: circularbuffer.New(7, 7),
		ohlcPeriod:    candleDuration,
	}
}

func (h *Harami) GetWarmUpCandleAmount() uint {
	return smaCandles
}

func (h *Harami) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	h.feedIndicator(closedCandle)
}

func (h *Harami) feedIndicator(closedCandle *ohlc.OHLC) {
	var high = helper.DecimalToFloat(closedCandle.High)
	var low = helper.DecimalToFloat(closedCandle.Low)
	h.sma.Insert(closedCandle)
	h.previousLows.Insert(low)
	h.previousHighs.Insert(high)
}

func (h *Harami) GetCandleDuration() time.Duration {
	return h.ohlcPeriod
}

func (h *Harami) isBearishCandle(candle *ohlc.OHLC) bool {
	return candle.Close.LessThan(candle.Open)
}

func (h *Harami) isBullishCandle(candle *ohlc.OHLC) bool {
	return candle.Close.GreaterThan(candle.Open)
}

func (h *Harami) isHaramiLong(firstCandle, secondCandle *ohlc.OHLC) bool {
	if h.isBearishCandle(firstCandle) && h.isBullishCandle(secondCandle) &&
		secondCandle.High.LessThan(firstCandle.Open) &&
		secondCandle.Low.GreaterThan(firstCandle.Close) {
		return true
	}
	return false
}

func (h *Harami) OnPosition(_, _ []broker.Position) {}

func (h *Harami) OnOrder(_ []broker.Order) {}

func (h *Harami) OnTick(_ tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	return
}

func (h *Harami) OnCandle(closedCandles []*ohlc.OHLC) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	var closedCandle = closedCandles[len(closedCandles)-1]
	var closePrice = helper.DecimalToFloat(closedCandle.Close)

	defer h.feedIndicator(closedCandle)

	smaValue, err := h.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)
		return
	}
	smaPrice := smaValue[sma.Value]

	if len(h.openPositions) > 0 {
		if closePrice < smaPrice {
			toClosePositions = h.openPositions
			return
		}

		previousCandlesHigh, err := h.previousHighs.Max()
		if err != nil {
			return
		}
		if helper.DecimalToFloat(closedCandle.Close) > previousCandlesHigh {
			toClosePositions = h.openPositions
			return
		}
		return
	}

	latestCandle := closedCandles[len(closedCandles)-1]
	secondLatestCandle := closedCandles[len(closedCandles)-2]
	if h.isHaramiLong(secondLatestCandle, latestCandle) {
		toOpenNew := h.prepareOrder(closedCandle, broker.BuyDirectionLong, 1.00)
		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}

	h.clog.Debug("no harami long candle found", "candle", closedCandle)
	return
}

func (h *Harami) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
	var (
		targetPrice   = helper.CalcTargetPriceByPercentage(closedCandle.Close, helper.FloatToDecimal(targetInPercent), direction)
		stopLossPrice = helper.CalcStopLossPriceByPercentage(closedCandle.Close, helper.FloatToDecimal(stopLossInPercent), direction)
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

func (h *Harami) Name() string {
	return strategy.NameHarami
}

func (h *Harami) String() string {
	return fmt.Sprintf("%s: Target=%.2f StopLoss=%.2f", h.Name(), targetInPercent, stopLossInPercent)
}
