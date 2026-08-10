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
)

func New(instrument string) *RSI {
	clog := slog.With("INSTRUMENT", instrument)

	return &RSI{
		clog:       clog,
		instrument: instrument,
		rsi:        stochrsi.New(5, 2, 14),
	}
}

func (d *RSI) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

func (d *RSI) OnTick(_ tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	return
}

func (d *RSI) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

func (d *RSI) GetCandleDuration() time.Duration {
	return ohlcPeriod
}

func (d *RSI) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.rsi.Insert(closedCandle)
}

func (d *RSI) GetWarmUpCandleAmount() uint {
	return 99
}

func (d *RSI) OnCandle(closedCandles []*ohlc.OHLC) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	if len(closedCandles) == 0 {
		return
	}
	closedCandle := closedCandles[len(closedCandles)-1]

	d.rsi.Insert(closedCandle)
	if len(d.openPositions) > 0 {
		return
	}

	rsiValueMap, err := d.rsi.Value()
	if err != nil {
		slog.Debug("Cannot get value from indicator", "error", err)
		return
	}
	kValue := rsiValueMap[stochrsi.ValueK]
	dValue := rsiValueMap[stochrsi.ValueD]
	if dValue == 0 {
		return
	}
	if kValue > upperThreshold && dValue > upperThreshold {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort, 1.00)
		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}
	if kValue < lowerThreshold && dValue < lowerThreshold {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong, 1.00)
		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}
	return
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

func (d *RSI) Name() string {
	return strategy.NameStochRSI
}

func (d *RSI) String() string {
	return d.Name()
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
	avg := (k + dv) / 2.0

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
