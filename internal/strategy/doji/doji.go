package doji

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/internal/broker"
	"github.com/tgragnato/orbiter/internal/strategy"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/tick"
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
	ohlcPeriod = time.Minute * 60
)

var (
	decZero         = decimal.NewFromFloat(0)
	targetInPercent = decimal.NewFromFloat(0.045)
	dec2Pip         = decimal.NewFromFloat(0.0002)
)

func New(instrument string) *Doji {
	clog := slog.With("INSTRUMENT", instrument)

	return &Doji{
		clog:       clog,
		instrument: instrument,
	}
}

func (d *Doji) OnWarmUpCandle(_ *ohlc.OHLC) {}

func (d *Doji) OnPosition(openPositions []broker.Position, _ []broker.Position) {
	d.openPositions = openPositions
}

func (d *Doji) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

func (d *Doji) GetCandleDuration() time.Duration {
	return ohlcPeriod
}

func (d *Doji) GetWarmUpCandleAmount() uint {
	return 1
}

// OnCandle records the latest closed candle for tick-level breakout detection.
func (d *Doji) OnCandle(closedCandles []*ohlc.OHLC) (toOpen []broker.Order, toClose []broker.Order, toClosePositions []broker.Position) {
	d.previousCandle = closedCandles[len(closedCandles)-1]
	return
}

// OnTick fires entry orders when the current tick breaks a preceding DOJI candle's range.
func (d *Doji) OnTick(currentTick tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	if len(d.openPositions) > 0 {
		return
	}
	if !isDOJI(d.previousCandle) {
		return
	}

	if currentTick.Bid.GreaterThan(d.previousCandle.High.Add(dec2Pip)) {
		order, err := d.createOrder(currentTick, targetInPercent, broker.BuyDirectionLong, 1.00)
		if err == nil {
			toOpen = []broker.Order{order}
		}
		return
	}

	if currentTick.Ask.LessThan(d.previousCandle.Low.Sub(dec2Pip)) {
		order, err := d.createOrder(currentTick, targetInPercent, broker.BuyDirectionShort, 1.00)
		if err == nil {
			toOpen = []broker.Order{order}
		}
		return
	}
	return
}

func (d *Doji) createOrder(currentTick tick.Tick, perfMargin decimal.Decimal, direction broker.BuyDirection, size float64) (broker.Order, error) {
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
	perfPercentage := candle.PerformanceInPercentage().Abs()
	return perfPercentage.LessThanOrEqual(decimal.NewFromFloat(0.025))
}

func (d *Doji) calcTargetPrice(direction broker.BuyDirection, t tick.Tick, perfMarginInPercentage decimal.Decimal) (decimal.Decimal, error) {
	switch direction {
	case broker.BuyDirectionLong:
		currentPrice := t.Ask
		percentFrom := currentPrice.Div(decimal.NewFromFloat(100)).Mul(perfMarginInPercentage)
		return currentPrice.Add(percentFrom).Round(6), nil
	case broker.BuyDirectionShort:
		currentPrice := t.Bid
		percentFrom := currentPrice.Div(decimal.NewFromFloat(100)).Mul(perfMarginInPercentage)
		return currentPrice.Sub(percentFrom).Round(6), nil
	default:
		return decZero, errors.New("unknown direction")
	}
}

func (d *Doji) calcStopLossPrice(direction broker.BuyDirection) (decimal.Decimal, error) {
	if d.previousCandle == nil {
		return decZero, errors.New("previousCandle is nil")
	}
	switch direction {
	case broker.BuyDirectionLong:
		return d.previousCandle.Low, nil
	case broker.BuyDirectionShort:
		return d.previousCandle.High, nil
	default:
		return decZero, errors.New("unknown direction")
	}
}

func (d *Doji) Name() string {
	return strategy.NameDOJI
}

func (d *Doji) String() string {
	return ""
}
