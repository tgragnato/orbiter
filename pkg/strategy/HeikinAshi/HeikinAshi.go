// Package heikinashi implements the Heikin-Ashi trading strategy.
package heikinashi

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/indicator"
	"github.com/tgragnato/orbiter/pkg/indicator/sma"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
	"github.com/tgragnato/orbiter/pkg/volatility"
)

const (
	volaShortPeriod      = 10
	volaLongPeriod       = 30
	smaPeriod            = 41
	minCandlesRequired   = 3
	candleAmountOffset   = 2
	volaQuantileLow      = 0.20
	volaQuantileMid      = 0.50
	volaQuantileHigh     = 0.95
	volaTargetMultiplier = 2.0
	percentDivisor       = 100.0
)

// Sentinel errors for the HeikinAshi strategy.
var (
	ErrNotEnoughClosedCandles    = errors.New("not enough closed candles to check")
	ErrUnexpectedCandleAmount    = errors.New("unexpected amount of candles")
	ErrNotEnoughCandlesDirection = errors.New("not enough candles in the right direction")
	ErrUnknownDirection          = errors.New("unknown direction")
)

// HeikinAshi implements the Heikin-Ashi trading strategy.
type HeikinAshi struct {
	clog                   *slog.Logger
	instrument             string
	closedHACandles        []*ohlc.OHLC
	ignoreInitialDirection bool
	initialDirection       *broker.BuyDirection
	currentDirection       *broker.BuyDirection
	volaTracker            *volatility.Volatility
	sma                    indicator.Indicator
	candlesReceived        bool
	openPositions          []broker.Position
	currentTick            tick.Tick
}

// New creates a new HeikinAshi strategy instance for the given instrument.
func New(instrument string) *HeikinAshi {
	clog := slog.With("INSTRUMENT", instrument)

	heikinAshi := &HeikinAshi{
		clog:                   clog,
		instrument:             instrument,
		closedHACandles:        nil,
		ignoreInitialDirection: true,
		initialDirection:       nil,
		currentDirection:       nil,
		volaTracker:            volatility.New(volaShortPeriod, volaLongPeriod),
		sma:                    sma.New(smaPeriod),
		candlesReceived:        false,
		openPositions:          nil,
		currentTick:            tick.Tick{ID: 0, Datetime: time.Time{}, Instrument: "", Bid: 0, Ask: 0},
	}

	return heikinAshi
}

// GetCandleDuration returns the candle duration used by this strategy.
func (ha *HeikinAshi) GetCandleDuration() time.Duration {
	return time.Hour
}

// GetWarmUpCandleAmount returns the number of candles required for warm-up.
func (ha *HeikinAshi) GetWarmUpCandleAmount() uint {
	return 1
}

// Name returns the strategy name.
func (ha *HeikinAshi) Name() string {
	return strategy.NameHeikinAshi
}

// OnCandle processes a new set of closed candles and returns orders.
//
//nolint:cyclop,funlen // signal logic complexity
func (ha *HeikinAshi) OnCandle(closedCandles []*ohlc.OHLC) ([]broker.Order, []broker.Order, []broker.Position) {
	var toOpen []broker.Order

	var toClose []broker.Order

	var toClosePositions []broker.Position

	closedCandle := closedCandles[len(closedCandles)-1]

	if ha.GetCandleDuration() == time.Hour*24 && closedCandle.Start.Weekday() == time.Sunday {
		return toOpen, toClose, toClosePositions
	}

	if ha.candlesReceived {
		ha.volaTracker.AddOHLC(closedCandle)
		ha.sma.Insert(closedCandle)
	} else {
		ha.candlesReceived = true
		for i := range closedCandles {
			ha.volaTracker.AddOHLC(closedCandles[i])
			ha.sma.Insert(closedCandles[i])
		}
	}

	if len(closedCandles) < minCandlesRequired {
		return toOpen, toClose, toClosePositions
	}

	haPrevious := ohlc.ToHeikinAshi(closedCandles[len(closedCandles)-3], closedCandles[len(closedCandles)-2])
	haNow := ohlc.ToHeikinAshi(closedCandles[len(closedCandles)-2], closedCandles[len(closedCandles)-1])
	ha.closedHACandles = append(ha.closedHACandles, haNow)

	var direction broker.BuyDirection

	switch {
	case isLongCandle(haNow) && isLongCandle(haPrevious) && haNow.Close > haPrevious.Close:
		direction = broker.BuyDirectionLong
	case isShortCandle(haNow) && isShortCandle(haPrevious) && haNow.Close < haPrevious.Close:
		direction = broker.BuyDirectionShort
	default:
		// undecided
		return toOpen, toClose, toClosePositions
	}

	defer func() {
		ha.currentDirection = &direction
	}()

	if ha.ignoreInitialDirection {
		if ha.initialDirection == nil {
			// Don't trade already running trend
			ha.initialDirection = &direction

			return toOpen, toClose, toClosePositions
		}

		if *ha.initialDirection != direction {
			ha.ignoreInitialDirection = false
			ha.initialDirection = nil
		}
	}

	havePositionInRightDirection := false

	for i := range ha.openPositions {
		position := ha.openPositions[i]
		if position.BuyDirection == direction {
			havePositionInRightDirection = true

			continue
		}

		toClosePositions = append(toClosePositions, position)
	}

	if havePositionInRightDirection {
		return toOpen, toClose, toClosePositions
	}

	// Open new positions only when the direction is changing
	if ha.currentDirection == nil || direction == *ha.currentDirection {
		return toOpen, toClose, toClosePositions
	}

	err := ha.checkCandleAmount(*ha.currentDirection, candleAmountOffset)
	if err != nil {
		ha.clog.Info("checkCandleAmount() failed", "error", err)

		return toOpen, toClose, toClosePositions
	}

	order, err := ha.createOrder(haNow, ha.currentTick, volaQuantileLow, direction)
	if err == nil {
		toOpen = append(toOpen, order)
	}

	order, err = ha.createOrder(haNow, ha.currentTick, volaQuantileMid, direction)
	if err == nil {
		toOpen = append(toOpen, order)
	}

	order, err = ha.createOrder(haNow, ha.currentTick, volaQuantileHigh, direction)
	if err == nil {
		toOpen = append(toOpen, order)
	}

	return toOpen, toClose, toClosePositions
}

// OnOrder handles order updates (no-op for this strategy).
func (ha *HeikinAshi) OnOrder(_ []broker.Order) {}

// OnPosition handles position updates (no-op for this strategy).
func (ha *HeikinAshi) OnPosition(_, _ []broker.Position) {}

// OnTick processes a new tick and returns orders.
//
func (ha *HeikinAshi) OnTick(currentTick tick.Tick) ([]broker.Order, []broker.Order, []broker.Position) {
	ha.currentTick = currentTick

	return nil, nil, nil
}

// OnWarmUpCandle processes a warm-up candle.
func (ha *HeikinAshi) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	ha.volaTracker.AddOHLC(closedCandle)
	ha.sma.Insert(closedCandle)
	ha.candlesReceived = true
}

// Score returns a continuous conviction in [-1.0, +1.0] based on the momentum
// of the last two Heikin-Ashi candles.
// The score is calculated based on the relative position of the current candle's
// close compared to the previous candle's close, normalized by the total range.
func (ha *HeikinAshi) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) < minCandlesRequired {
		return 0
	}

	haNow := ohlc.ToHeikinAshi(closedCandles[len(closedCandles)-2], closedCandles[len(closedCandles)-1])
	haPrev := ohlc.ToHeikinAshi(closedCandles[len(closedCandles)-3], closedCandles[len(closedCandles)-2])

	// Calculate the total range of the two candles
	rangePrev := haPrev.High - haPrev.Low
	rangeNow := haNow.High - haNow.Low
	totalRange := rangePrev + rangeNow

	if totalRange == 0 {
		return 0
	}

	// Calculate the difference between the current and previous close
	diff := haNow.Close - haPrev.Close

	// Calculate the normalized position of the current close within the combined range.
	// Score = diff / totalRange
	score := diff / totalRange

	if score > 1.0 {
		return 1.0
	}

	if score < -1.0 {
		return -1.0
	}

	return score
}

// String returns the strategy name.
func (ha *HeikinAshi) String() string {
	return strategy.NameHeikinAshi
}

func (ha *HeikinAshi) calcStopLossPrice(direction broker.BuyDirection, currentTick tick.Tick) (float64, error) {
	volaMaxFloat, err := ha.volaTracker.VolatilityInPercentageQuantile(volaQuantileHigh)
	if err != nil {
		return 0, fmt.Errorf("VolatilityInPercentageQuantile() failed: %w", err)
	}

	volaMax := math.Abs(volaMaxFloat)

	switch direction {
	case broker.BuyDirectionLong:
		return currentTick.Price() - volaMax, nil
	case broker.BuyDirectionShort:
		return currentTick.Price() + volaMax, nil
	default:
		return 0, ErrUnknownDirection
	}
}

func (ha *HeikinAshi) calcTargetPrice(
	direction broker.BuyDirection,
	currentTick tick.Tick,
	perfMarginInPercentage float64,
) (float64, error) {
	switch direction {
	case broker.BuyDirectionLong:
		currentPrice := currentTick.Ask
		percentFrom := currentPrice * perfMarginInPercentage / percentDivisor

		return currentPrice + percentFrom, nil
	case broker.BuyDirectionShort:
		currentPrice := currentTick.Bid
		percentFrom := currentPrice * perfMarginInPercentage / percentDivisor

		return currentPrice - percentFrom, nil
	default:
		return 0, ErrUnknownDirection
	}
}

func (ha *HeikinAshi) checkCandleAmount(direction broker.BuyDirection, offset int) error {
	const candlesToCheck = 4

	maxVal := candlesToCheck + offset
	lenCandles := len(ha.closedHACandles)

	if lenCandles < maxVal {
		return ErrNotEnoughClosedCandles
	}

	candles := ha.closedHACandles[lenCandles-maxVal : lenCandles-offset]
	if len(candles) != candlesToCheck {
		return fmt.Errorf("%w: %d", ErrUnexpectedCandleAmount, len(candles))
	}

	candlesInDirection := 0

	for _, candle := range candles {
		var candleDirection broker.BuyDirection
		if candle.Close >= candle.Open {
			candleDirection = broker.BuyDirectionLong
		} else {
			candleDirection = broker.BuyDirectionShort
		}

		if candleDirection == direction {
			candlesInDirection++
		}
	}

	if candlesInDirection < candlesToCheck {
		return fmt.Errorf("%w (%s), need %d, found %d",
			ErrNotEnoughCandlesDirection, direction.String(), candlesToCheck, candlesInDirection)
	}

	return nil
}

func (ha *HeikinAshi) createOrder(
	haCandle *ohlc.OHLC,
	currentTick tick.Tick,
	volaQuantileForTarget float64,
	direction broker.BuyDirection,
) (broker.Order, error) {
	const size = 1

	volaTargetFloat, err := ha.volaTracker.VolatilityInPercentageQuantile(volaQuantileForTarget)
	if err != nil {
		return broker.Order{}, fmt.Errorf("VolatilityInPercentageQuantile() failed: %w", err)
	}

	volaTarget := math.Abs(volaTargetFloat) * volaTargetMultiplier

	targetPrice, err := ha.calcTargetPrice(direction, currentTick, volaTarget)
	if err != nil {
		return broker.Order{}, fmt.Errorf("calcTargetPrice() failed: %w", err)
	}

	stopLossPrice, err := ha.calcStopLossPrice(direction, currentTick)
	if err != nil {
		return broker.Order{}, fmt.Errorf("calcStopLossPrice() failed: %w", err)
	}

	ha.clog.Debug("Creating new order",
		"Direction", direction.String(),
		"Time", currentTick.Datetime,
		"CurrentTick.Bid", currentTick.Bid,
		"CurrentTick.Ask", currentTick.Ask,
		"VolaTarget", volaTarget,
		"TargetPrice", targetPrice,
		"StopLossPrice", stopLossPrice,
		"OHLC.Age", haCandle.Age(currentTick.Datetime).String(),
	)

	return broker.NewMarketOrder(direction, size, haCandle.Instrument, targetPrice, stopLossPrice), nil
}

func isLongCandle(candle *ohlc.OHLC) bool {
	return candle.Close > candle.Open
}

func isShortCandle(candle *ohlc.OHLC) bool {
	return candle.Close < candle.Open
}
