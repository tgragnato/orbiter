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
	strategyLongEnabled  = true
	strategyShortEnabled = true
)

func New(instrument string, candleDuration time.Duration) *SMA {
	clog := slog.With("INSTRUMENT", instrument, "CANDLE", candleDuration)

	return &SMA{
		clog:       clog,
		instrument: instrument,
		sma:        sma.New(smaCandles),
		sma10:      sma.New(10),
		ohlcPeriod: candleDuration,
	}
}

func (d *SMA) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

func (d *SMA) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

func (d *SMA) GetWarmUpCandleAmount() uint {
	return smaCandles
}

func (d *SMA) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.feedIndicator(closedCandle)
}

func (d *SMA) feedIndicator(closedCandle *ohlc.OHLC) {
	d.sma.Insert(closedCandle)
	d.sma10.Insert(closedCandle)
}

func (d *SMA) GetCandleDuration() time.Duration {
	return d.ohlcPeriod
}

func (d *SMA) OnTick(_ tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	return
}

func (d *SMA) OnCandle(closedCandles []*ohlc.OHLC) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
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

func (d *SMA) strategyLong(closedCandles []*ohlc.OHLC) (toOpen []broker.Order, toClose []broker.Position) {
	var closedCandle = closedCandles[len(closedCandles)-1]
	var closePrice = helper.DecimalToFloat(closedCandle.Close)

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)
		return
	}
	sma200Price := smaValue[sma.Value]

	sma10, err := d.sma10.Value()
	if err != nil {
		return
	}
	sma10Price := sma10[sma.Value]

	for i := range d.openPositions {
		if d.openPositions[i].BuyDirection != broker.BuyDirectionLong {
			continue
		}
		if helper.DecimalToFloat(closedCandle.Close) > sma10Price {
			toClose = d.openPositions
			return
		}
	}
	if len(d.openPositions) > 0 {
		return
	}

	if closePrice < sma200Price {
		d.clog.Debug("close is below SMA200", "close", closePrice, "sma200", sma200Price)
		return
	}

	if closePrice < sma10Price {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong, 1.00)
		return []broker.Order{toOpenNew}, []broker.Position{}
	}
	d.clog.Debug("long: closePrice >= SMA10", "close", closePrice, "sma10", sma10Price)
	return
}

func (d *SMA) strategyShort(closedCandles []*ohlc.OHLC) (toOpen []broker.Order, toClose []broker.Position) {
	var closedCandle = closedCandles[len(closedCandles)-1]
	var closePrice = helper.DecimalToFloat(closedCandle.Close)

	smaValue, err := d.sma.Value()
	if err != nil {
		slog.Warn("No SMA", "error", err)
		return
	}
	sma200Price := smaValue[sma.Value]

	sma10, err := d.sma10.Value()
	if err != nil {
		return
	}
	sma10Price := sma10[sma.Value]

	for i := range d.openPositions {
		if d.openPositions[i].BuyDirection != broker.BuyDirectionShort {
			continue
		}
		if helper.DecimalToFloat(closedCandle.Close) < sma10Price {
			toClose = d.openPositions
			return
		}
	}
	if len(d.openPositions) > 0 {
		return
	}

	if closePrice > sma200Price {
		d.clog.Debug("close is above SMA200", "close", closePrice, "sma200", sma200Price)
		return
	}

	if closePrice > sma10Price {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort, 1.00)
		return []broker.Order{toOpenNew}, []broker.Position{}
	}
	d.clog.Debug("short: closePrice <= SMA10", "close", closePrice, "sma10", sma10Price)
	return
}

func (d *SMA) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
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

func (d *SMA) Name() string {
	return strategy.NameSMA10
}

func (d *SMA) String() string {
	return fmt.Sprintf("%s: Long=%t, Short=%t Target=%.2f%% StopLoss=%.2f%% SMA%d", d.Name(),
		strategyLongEnabled, strategyShortEnabled, targetInPercent, stopLossInPercent, smaCandles)
}

// Score returns a directional conviction in [-1.0, +1.0].
// Close above SMA-200 and below SMA-10 → positive (buy setup).
// Close below SMA-200 and above SMA-10 → negative (sell setup).
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

	closePrice := helper.DecimalToFloat(closedCandles[len(closedCandles)-1].Close)

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
