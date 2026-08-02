package lowcandle

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/internal/broker"
	"github.com/tgragnato/orbiter/internal/strategy"
	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// Long: Buy if candle closes below the last 7 candles and is above SMA 200
// Short: Short if candle closes above the last 7 candles and is below SMA 200
// Source: https://www.youtube.com/watch?v=_9Bmxylp63Y

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
)

func New(instrument string, candleDuration time.Duration) *LowCandle {
	clog := slog.With("INSTRUMENT", instrument, "CANDLE", candleDuration)

	return &LowCandle{
		clog:          clog,
		instrument:    instrument,
		sma:           sma.New(smaCandles),
		previousLows:  circularbuffer.New(7, 7),
		previousHighs: circularbuffer.New(7, 7),
		ohlcPeriod:    candleDuration,
	}
}

func (d *LowCandle) OnPosition(openPositions []broker.Position, _ []broker.Position) {
	d.openPositions = openPositions
}

func (d *LowCandle) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

func (d *LowCandle) GetWarmUpCandleAmount() uint {
	return smaCandles
}

func (d *LowCandle) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.feedIndicator(closedCandle)
}

func (d *LowCandle) feedIndicator(closedCandle *ohlc.OHLC) {
	var high = helper.DecimalToFloat(closedCandle.High)
	var low = helper.DecimalToFloat(closedCandle.Low)
	d.sma.Insert(closedCandle)
	d.previousLows.Insert(low)
	d.previousHighs.Insert(high)
}

func (d *LowCandle) GetCandleDuration() time.Duration {
	return d.ohlcPeriod
}

func (d *LowCandle) OnTick(_ tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	return
}

func (d *LowCandle) OnCandle(closedCandles []*ohlc.OHLC) (toOpen []broker.Order, toClose []broker.Order, toClosePositions []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]
	defer d.feedIndicator(closedCandle)

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
	return
}

func (d *LowCandle) strategyLong(closedCandles []*ohlc.OHLC) (toOpen []broker.Order, toClose []broker.Position) {
	var closedCandle = closedCandles[len(closedCandles)-1]
	var closePrice = helper.DecimalToFloat(closedCandle.Close)
	var lowPrice = helper.DecimalToFloat(closedCandle.Low)

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)
		return
	}
	smaPrice := smaValue[sma.Value]

	for _, openPosition := range d.openPositions {
		if openPosition.BuyDirection != broker.BuyDirectionLong {
			continue
		}
		previousCandlesHigh, err := d.previousHighs.Max()
		if err != nil {
			return
		}
		if helper.DecimalToFloat(closedCandle.Close) > previousCandlesHigh {
			toClose = d.openPositions
			return
		}
	}
	if len(d.openPositions) > 0 {
		return
	}

	if closePrice < smaPrice && lowPrice < smaPrice {
		d.clog.Debug("close is below SMA", "close", closePrice, "sma", smaPrice)
		return
	}

	previousCandlesLow, err := d.previousLows.Min()
	if err != nil {
		d.clog.Warn("no previous low", "error", err)
		return
	}

	if closePrice < previousCandlesLow {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong, 1.00)
		return []broker.Order{toOpenNew}, []broker.Position{}
	}
	d.clog.Debug("long: closePrice >= previousCandlesLow", "close", closePrice, "previousLow", previousCandlesLow)
	return
}

func (d *LowCandle) strategyShort(closedCandles []*ohlc.OHLC) (toOpen []broker.Order, toClose []broker.Position) {
	var closedCandle = closedCandles[len(closedCandles)-1]
	var closePrice = helper.DecimalToFloat(closedCandle.Close)
	var highPrice = helper.DecimalToFloat(closedCandle.High)

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)
		return
	}
	smaPrice := smaValue[sma.Value]

	for _, openPosition := range d.openPositions {
		if openPosition.BuyDirection != broker.BuyDirectionShort {
			continue
		}
		previousCandlesLow, err := d.previousLows.Min()
		if err != nil {
			return
		}
		if helper.DecimalToFloat(closedCandle.Close) < previousCandlesLow {
			toClose = append(toClose, openPosition)
			return
		}
	}
	if len(d.openPositions) > 0 {
		return
	}

	if closePrice > smaPrice && highPrice > smaPrice {
		d.clog.Debug("close is above SMA", "close", closePrice, "sma", smaPrice)
		return
	}

	previousCandlesHigh, err := d.previousHighs.Max()
	if err != nil {
		d.clog.Warn("no previous high", "error", err)
		return
	}

	if closePrice > previousCandlesHigh {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort, 1.00)
		return []broker.Order{toOpenNew}, []broker.Position{}
	}
	d.clog.Debug("short: closePrice <= previousCandlesHigh", "close", closePrice, "previousHigh", previousCandlesHigh)
	return
}

func (d *LowCandle) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
	var (
		targetPrice   = helper.CalcTargetPriceByPercentage(closedCandle.Close, helper.FloatToDecimal(targetInPercent), direction)
		stopLossPrice = helper.CalcStopLossPriceByPercentage(closedCandle.Close, helper.FloatToDecimal(stopLossInPercent), direction)
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

func (d *LowCandle) Name() string {
	return strategy.NameLowCandle
}

func (d *LowCandle) String() string {
	return fmt.Sprintf("%s: Long=%t, Short=%t Target=%.2f%% StopLoss=%.2f%% SMA%d", d.Name(),
		strategyLongEnabled, strategyShortEnabled, targetInPercent, stopLossInPercent, smaCandles)
}
