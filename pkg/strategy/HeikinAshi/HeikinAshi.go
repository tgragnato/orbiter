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

func New(instrument string) *HeikinAshi {
	clog := slog.With("INSTRUMENT", instrument)

	ha := &HeikinAshi{
		clog:                   clog,
		instrument:             instrument,
		ignoreInitialDirection: true,
		volaTracker:            volatility.New(10, 30),
		sma:                    sma.New(41),
	}

	return ha
}

func (ha *HeikinAshi) GetCandleDuration() time.Duration {
	return time.Hour
}

func (ha *HeikinAshi) GetWarmUpCandleAmount() uint {
	return 1
}

func (ha *HeikinAshi) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	ha.volaTracker.AddOHLC(closedCandle)
	ha.sma.Insert(closedCandle)
	ha.candlesReceived = true
}

func (ha *HeikinAshi) OnPosition(_, _ []broker.Position) {}
func (ha *HeikinAshi) OnOrder(_ []broker.Order)          {}

func (ha *HeikinAshi) OnTick(currentTick tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	ha.currentTick = currentTick
	return
}

func (ha *HeikinAshi) OnCandle(closedCandles []*ohlc.OHLC) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]

	if ha.GetCandleDuration() == time.Hour*24 && closedCandle.Start.Weekday() == time.Sunday {
		return
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

	if len(closedCandles) < 3 {
		return
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
		return
	}

	defer func() {
		ha.currentDirection = &direction
	}()

	if ha.ignoreInitialDirection {
		if ha.initialDirection == nil {
			// Don't trade already running trend
			ha.initialDirection = &direction
			return
		}
		if *ha.initialDirection != direction {
			ha.ignoreInitialDirection = false
			ha.initialDirection = nil
		}
	}

	var havePositionInRightDirection = false
	for i := range ha.openPositions {
		position := ha.openPositions[i]
		if position.BuyDirection == direction {
			havePositionInRightDirection = true
			continue
		}
		toClosePositions = append(toClosePositions, position)
	}
	if havePositionInRightDirection {
		return
	}

	// Open new positions only when the direction is changing
	if ha.currentDirection == nil || direction == *ha.currentDirection {
		return
	}

	if err := ha.checkCandleAmount(*ha.currentDirection, 2); err != nil {
		ha.clog.Info("checkCandleAmount() failed", "error", err)
		return
	}

	order, err := ha.createOrder(haNow, ha.currentTick, 0.20, direction)
	if err == nil {
		toOpen = append(toOpen, order)
	}
	order, err = ha.createOrder(haNow, ha.currentTick, 0.50, direction)
	if err == nil {
		toOpen = append(toOpen, order)
	}
	order, err = ha.createOrder(haNow, ha.currentTick, 0.95, direction)
	if err == nil {
		toOpen = append(toOpen, order)
	}

	return
}

func (ha *HeikinAshi) createOrder(haCandle *ohlc.OHLC, currentTick tick.Tick, volaQuantileForTarget float64, direction broker.BuyDirection) (broker.Order, error) {
	const size = 1

	volaTargetFloat, err := ha.volaTracker.VolatilityInPercentageQuantile(volaQuantileForTarget)
	if err != nil {
		return broker.Order{}, err
	}
	volaTarget := math.Abs(volaTargetFloat) * 2

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

func isShortCandle(candle *ohlc.OHLC) bool {
	return candle.Close < candle.Open
}

func isLongCandle(candle *ohlc.OHLC) bool {
	return candle.Close > candle.Open
}

func (ha *HeikinAshi) checkCandleAmount(direction broker.BuyDirection, offset int) error {
	const candlesToCheck = 4
	maxVal := candlesToCheck + offset
	lenCandles := len(ha.closedHACandles)

	if lenCandles < maxVal {
		return errors.New("not enough closed candles to check")
	}
	candles := ha.closedHACandles[lenCandles-maxVal : lenCandles-offset]
	if len(candles) != candlesToCheck {
		return fmt.Errorf("unexecpted amount of candles: %d", len(candles))
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
		return fmt.Errorf("not enough candles in the right direction (%s), need %d, found %d",
			direction.String(), candlesToCheck, candlesInDirection)
	}
	return nil
}

func (ha *HeikinAshi) calcTargetPrice(direction broker.BuyDirection, t tick.Tick, perfMarginInPercentage float64) (float64, error) {
	switch direction {
	case broker.BuyDirectionLong:
		var currentPrice = t.Ask
		percentFrom := currentPrice * perfMarginInPercentage / 100
		return currentPrice + percentFrom, nil
	case broker.BuyDirectionShort:
		var currentPrice = t.Bid
		percentFrom := currentPrice * perfMarginInPercentage / 100
		return currentPrice - percentFrom, nil
	default:
		return 0, errors.New("unknown direction")
	}
}

func (ha *HeikinAshi) calcStopLossPrice(direction broker.BuyDirection, t tick.Tick) (float64, error) {
	volaMaxFloat, err := ha.volaTracker.VolatilityInPercentageQuantile(0.95)
	if err != nil {
		return 0, err
	}
	volaMax := math.Abs(volaMaxFloat)

	switch direction {
	case broker.BuyDirectionLong:
		return t.Price() - volaMax, nil
	case broker.BuyDirectionShort:
		return t.Price() + volaMax, nil
	default:
		return 0, errors.New("unknown direction")
	}
}

func (ha *HeikinAshi) Name() string {
	return strategy.NameHeikinAshi
}

func (ha *HeikinAshi) String() string {
	return strategy.NameHeikinAshi
}

// Score returns a continuous conviction in [-1.0, +1.0] based on the momentum
// of the last two Heikin-Ashi candles.
// The score is calculated based on the relative position of the current candle's
// close compared to the previous candle's close, normalized by the total range.
func (ha *HeikinAshi) Score(closedCandles []*ohlc.OHLC) float64 {
	if len(closedCandles) < 3 {
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

	// Calculate the normalized position of the current close within the combined range
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
