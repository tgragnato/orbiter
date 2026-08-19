package scalper

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// scalper targets a small profit.
// Entry: Do counter trade after a number of candles were in the same buy direction.
type scalper struct {
	clog          *slog.Logger
	openPositions []broker.Position
	openOrders    []broker.Order
	currentTick   tick.Tick
}

const (
	targetPercent   = 0.12
	stopLossPercent = 0.25
	candleDuration  = 5
	percentDivisor  = 100.0
)

// New creates a new scalper strategy for the given instrument.
func New(instrument string) *scalper {
	clog := slog.With("INSTRUMENT", instrument)

	scalperInst := &scalper{
		clog: clog,
		currentTick: tick.Tick{
			ID:         0,
			Datetime:   time.Time{},
			Instrument: "",
			Bid:        0,
			Ask:        0,
		},
		openPositions: nil,
		openOrders:    nil,
	}

	return scalperInst
}

func (mr *scalper) GetCandleDuration() time.Duration {
	return time.Minute * candleDuration
}

func (mr *scalper) GetWarmUpCandleAmount() uint {
	return 1
}

func (mr *scalper) Name() string {
	return strategy.NameScalper
}

func (mr *scalper) OnCandle(closedCandles []*ohlc.OHLC) ([]broker.Order, []broker.Order, []broker.Position) {
	const candles = 10

	if len(mr.openPositions) > 0 {
		return nil, nil, nil
	}

	if len(closedCandles) < candles {
		return nil, nil, nil
	}

	closedCandle := closedCandles[len(closedCandles)-1]

	buyDirection := getBuyDirection(closedCandle)

	for i := len(closedCandles) - candles; i < len(closedCandles)-1; i++ {
		candleDirection := getBuyDirection(closedCandles[i])
		if candleDirection == buyDirection {
			// Candles before closedCandle needs to have a different direction.
			return nil, nil, nil
		}
	}

	newOrder, err := mr.createOrder(closedCandle, buyDirection, 1)
	if err != nil {
		mr.clog.Error("createOrder() failed", "error", err)

		return nil, nil, nil
	}

	return []broker.Order{newOrder}, nil, nil
}

func (mr *scalper) OnOrder(openOrders []broker.Order) {
	mr.openOrders = openOrders
}

func (mr *scalper) OnPosition(openPositions, _ []broker.Position) {
	mr.openPositions = openPositions
}

func (mr *scalper) OnTick(currentTick tick.Tick) ([]broker.Order, []broker.Order, []broker.Position) {
	mr.currentTick = currentTick

	return nil, nil, nil
}

func (mr *scalper) OnWarmUpCandle(_ *ohlc.OHLC) {}

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

	// Check all prior candles.
	for idx := range total - 1 {
		if getBuyDirection(closedCandles[idx]) != lastDir {
			oppositeCount++
		}
	}

	// Calculate continuous score: (opposite count / total-1) * sign.
	score := float64(oppositeCount) / float64(total-1)

	if lastDir == broker.BuyDirectionLong {
		// Last candle long, opposite candles are short -> buy signal (positive score).
		return score
	}

	// Last candle short, opposite candles are long -> sell signal (negative score).
	return -score
}

func (mr *scalper) String() string {
	return mr.Name()
}

func (mr *scalper) calcStopLossPrice(
	direction broker.BuyDirection,
	currentTick tick.Tick,
	percentage float64,
) (float64, error) {
	switch direction {
	case broker.BuyDirectionLong:
		currentPrice := currentTick.Ask
		percentFrom := currentPrice / percentDivisor * percentage

		return currentPrice - percentFrom, nil
	case broker.BuyDirectionShort:
		currentPrice := currentTick.Bid
		percentFrom := currentPrice / percentDivisor * percentage

		return currentPrice + percentFrom, nil
	default:
		return 0, broker.ErrUnknownBuyDirection
	}
}

func (mr *scalper) calcTargetPrice(
	direction broker.BuyDirection,
	currentTick tick.Tick,
	percentage float64,
) (float64, error) {
	switch direction {
	case broker.BuyDirectionLong:
		currentPrice := currentTick.Ask
		percentFrom := currentPrice / percentDivisor * percentage

		return currentPrice + percentFrom, nil
	case broker.BuyDirectionShort:
		currentPrice := currentTick.Bid
		percentFrom := currentPrice / percentDivisor * percentage

		return currentPrice - percentFrom, nil
	default:
		return 0, broker.ErrUnknownBuyDirection
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

func getBuyDirection(candle *ohlc.OHLC) broker.BuyDirection {
	if candle.PerformanceInPercentage() >= 0 {
		return broker.BuyDirectionLong
	}

	return broker.BuyDirectionShort
}
