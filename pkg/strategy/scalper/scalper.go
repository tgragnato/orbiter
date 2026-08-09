package scalper

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/shopspring/decimal"
	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// scalper targets a small profit
// Entry: Do counter trade after a number of candles were in the same buy direction

type scalper struct {
	clog          *slog.Logger
	openPositions []broker.Position
	openOrders    []broker.Order
	currentTick   tick.Tick
}

var (
	targetPercent   = decimal.NewFromFloat(0.12)
	stopLossPercent = decimal.NewFromFloat(0.25)
)

func New(instrument string) *scalper {
	clog := slog.With("INSTRUMENT", instrument)

	mr := &scalper{
		clog: clog,
	}

	return mr
}

func (mr *scalper) OnPosition(openPositions, _ []broker.Position) {
	mr.openPositions = openPositions
}

func (mr *scalper) OnOrder(openOrders []broker.Order) {
	mr.openOrders = openOrders
}

func (mr *scalper) OnWarmUpCandle(_ *ohlc.OHLC) {}

func (mr *scalper) GetWarmUpCandleAmount() uint {
	return 1
}

func (mr *scalper) GetCandleDuration() time.Duration {
	return time.Minute * 5
}

func (mr *scalper) OnTick(currentTick tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	mr.currentTick = currentTick
	return
}

func (mr *scalper) OnCandle(closedCandles []*ohlc.OHLC) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	const candles = 10

	if len(mr.openPositions) > 0 {
		return
	}
	if len(closedCandles) < candles {
		return
	}

	closedCandle := closedCandles[len(closedCandles)-1]

	var buyDirection = getBuyDirection(closedCandle)
	for i := len(closedCandles) - candles; i < len(closedCandles)-1; i++ {
		candleDirection := getBuyDirection(closedCandles[i])
		if candleDirection == buyDirection {
			// Candles before closedCandle needs to have a different direction
			return
		}
	}

	newOrder, err := mr.createOrder(closedCandle, buyDirection, 1)
	if err != nil {
		mr.clog.Error("createOrder() failed", "error", err)
		return
	}
	toOpen = append(toOpen, newOrder)

	return
}

func getBuyDirection(candle *ohlc.OHLC) broker.BuyDirection {
	if candle.PerformanceInPercentage().GreaterThanOrEqual(decimal.Zero) {
		return broker.BuyDirectionLong
	} else {
		return broker.BuyDirectionShort
	}
}

func (mr *scalper) createOrder(openOHLC *ohlc.OHLC, direction broker.BuyDirection, size float64) (broker.Order, error) {
	targetPrice, err := mr.calcTargetPrice(direction, mr.currentTick, targetPercent)
	if err != nil {
		return broker.Order{}, fmt.Errorf("calcTargetPrice() failed: %w", err)
	}

	stopLossPrice, err := mr.calcStopLossPrice(direction, mr.currentTick, stopLossPercent)
	if err != nil {
		return broker.Order{}, fmt.Errorf("calcStopLossPrice() failed: %w", err)
	}

	mr.clog.Debug("Creating new order",
		"Direction", direction.String(),
		"Time", mr.currentTick.Datetime,
		"CurrentTick.Bid", mr.currentTick.Bid,
		"CurrentTick.Ask", mr.currentTick.Ask,
		"TargetPrice", targetPrice,
		"StopLossPrice", stopLossPrice,
		"OHLC.Age", openOHLC.Age(mr.currentTick.Datetime).String(),
	)

	return broker.NewMarketOrder(direction, size, openOHLC.Instrument, targetPrice, stopLossPrice), nil
}

func (mr *scalper) calcTargetPrice(direction broker.BuyDirection, t tick.Tick, percentage decimal.Decimal) (decimal.Decimal, error) {
	switch direction {
	case broker.BuyDirectionLong:
		var currentPrice = t.Ask
		percentFrom := currentPrice.Div(decimal.NewFromFloat(100)).Mul(percentage)
		return currentPrice.Add(percentFrom).Round(6), nil
	case broker.BuyDirectionShort:
		var currentPrice = t.Bid
		percentFrom := currentPrice.Div(decimal.NewFromFloat(100)).Mul(percentage)
		return currentPrice.Sub(percentFrom).Round(6), nil
	default:
		return decimal.Zero, broker.ErrUnknownBuyDirection
	}
}

func (mr *scalper) calcStopLossPrice(direction broker.BuyDirection, t tick.Tick, percentage decimal.Decimal) (decimal.Decimal, error) {
	switch direction {
	case broker.BuyDirectionLong:
		var currentPrice = t.Ask
		percentFrom := currentPrice.Div(decimal.NewFromFloat(100)).Mul(percentage)
		return currentPrice.Sub(percentFrom).Round(6), nil
	case broker.BuyDirectionShort:
		var currentPrice = t.Bid
		percentFrom := currentPrice.Div(decimal.NewFromFloat(100)).Mul(percentage)
		return currentPrice.Add(percentFrom).Round(6), nil
	default:
		return decimal.Zero, broker.ErrUnknownBuyDirection
	}
}

func (mr *scalper) Name() string {
	return strategy.NameScalper
}

func (mr *scalper) String() string {
	return mr.Name()
}

// Score returns a continuous conviction in [-1.0, +1.0] based on the number of
// prior candles that are in the opposite direction of the last candle.
// The score is calculated as (number of opposite candles / 9.0) * sign.
func (mr *scalper) Score(closedCandles []*ohlc.OHLC) float64 {
	total := len(closedCandles)
	if total <= 1 {
		return 0
	}

	last := closedCandles[total-1]
	lastDir := getBuyDirection(last)
	oppositeCount := 0

	// Check all prior candles
	for i := 0; i < total-1; i++ {
		if getBuyDirection(closedCandles[i]) != lastDir {
			oppositeCount++
		}
	}

	// Calculate continuous score: (opposite count / total-1) * sign
	score := float64(oppositeCount) / float64(total-1)
	if lastDir == broker.BuyDirectionLong {
		// Last candle long, opposite candles are short -> buy signal (positive score)
		return score
	}
	// Last candle short, opposite candles are long -> sell signal (negative score)
	return -score
}
