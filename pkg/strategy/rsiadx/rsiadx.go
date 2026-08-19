// Package rsiadx implements a strategy combining the RSI and ADX indicators.
package rsiadx

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/tgragnato/orbiter/pkg/broker"
	"github.com/tgragnato/orbiter/pkg/eo"
	"github.com/tgragnato/orbiter/pkg/helper"
	indicatoradx "github.com/tgragnato/orbiter/pkg/indicator/adx"
	indicatorrsi "github.com/tgragnato/orbiter/pkg/indicator/rsi"
	"github.com/tgragnato/orbiter/pkg/ohlc"
	"github.com/tgragnato/orbiter/pkg/strategy"
	"github.com/tgragnato/orbiter/pkg/tick"
)

// RSIADX is a strategy that combines the RSI and ADX indicators.
type RSIADX struct {
	clog           *slog.Logger
	instrument     string
	rsi            *indicatorrsi.RSI
	adx            *indicatoradx.ADX
	candleDuration time.Duration
	eo             *eo.EnvironmentOverlay
	openPositions  []broker.Position
	openOrders     []broker.Order
}

const (
	adxThreshold         = 35
	adxCandles           = 10
	rsiCandles           = 2
	targetInPercent      = 5.0
	stopLossInPercent    = 2.5
	maxAgeOpenPosition   = time.Hour * 2
	fullSize             = 1.00
	warmUpMultiplier     = 2
	adxScaleCeiling      = 100.0
	rsiMidpoint          = 50.0
)

// New creates a new RSIADX strategy instance.
func New(instrument string, candleDuration time.Duration) *RSIADX {
	clog := slog.With("INSTRUMENT", instrument)

	return &RSIADX{
		clog:           clog,
		instrument:     instrument,
		rsi:            indicatorrsi.New(rsiCandles),
		adx:            indicatoradx.New(adxCandles),
		candleDuration: candleDuration,
		eo:             eo.New(),
		openPositions:  []broker.Position{},
		openOrders:     []broker.Order{},
	}
}

// GetCandleDuration returns the candle duration for this strategy.
func (d *RSIADX) GetCandleDuration() time.Duration {
	return d.candleDuration
}

// GetWarmUpCandleAmount returns the number of warm-up candles needed.
func (d *RSIADX) GetWarmUpCandleAmount() uint {
	return adxCandles * warmUpMultiplier
}

// Name returns the strategy name.
func (d *RSIADX) Name() string {
	return strategy.NameRSIADX
}

// OnCandle processes a new closed candle and returns orders/positions to manage.
func (d *RSIADX) OnCandle(
	closedCandles []*ohlc.OHLC,
) ([]broker.Order, []broker.Order, []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]

	d.rsi.Insert(closedCandle)
	d.adx.Insert(closedCandle)
	d.eo.AddCandle(closedCandle)

	if len(d.openPositions) > 0 {
		return d.checkOpenPositions(closedCandle, closedCandles, d.openPositions)
	}

	if !d.isStrongADXTrend() {
		return nil, nil, nil
	}

	if d.isRSIShortSignal() {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort, fullSize)

		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	} else if d.isRSILongSignal() {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong, fullSize)

		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}

	return nil, nil, nil
}

// OnOrder updates the open orders.
func (d *RSIADX) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

// OnPosition updates the open positions.
func (d *RSIADX) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

// OnTick handles a new tick (no-op for this strategy).
func (d *RSIADX) OnTick(
	_ tick.Tick,
) ([]broker.Order, []broker.Order, []broker.Position) {
	return nil, nil, nil
}

// OnWarmUpCandle feeds a warm-up candle into the indicators.
func (d *RSIADX) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.rsi.Insert(closedCandle)
	d.adx.Insert(closedCandle)
	d.eo.AddCandle(closedCandle)
}

// Score returns a conviction in [-1.0, +1.0]. Without a strong ADX trend the
// score is zero. With ADX >= threshold the RSI conviction is scaled by the
// normalised ADX strength (capped at 1.0).
func (d *RSIADX) Score(_ []*ohlc.OHLC) float64 {
	adxVal, err := d.getADX()
	if err != nil || adxVal < adxThreshold {
		return 0
	}

	rsiVal, err := d.getRSI()
	if err != nil {
		return 0
	}

	upper, lower := d.eo.RSI()

	// Normalise ADX strength relative to threshold (100 as ceiling).
	adxScale := (adxVal - adxThreshold) / (adxScaleCeiling - adxThreshold)
	if adxScale > 1 {
		adxScale = 1
	}

	var rsiConviction float64

	switch {
	case rsiVal <= lower:
		rsiConviction = 1.0
	case rsiVal >= upper:
		rsiConviction = -1.0
	case rsiVal < rsiMidpoint:
		rsiConviction = (rsiMidpoint - rsiVal) / (rsiMidpoint - lower)
	default:
		rsiConviction = -(rsiVal - rsiMidpoint) / (upper - rsiMidpoint)
	}

	return rsiConviction * adxScale
}

// String returns the string representation of the strategy.
func (d *RSIADX) String() string {
	return d.Name()
}

func (d *RSIADX) checkOpenPositions(
	closedCandle *ohlc.OHLC,
	closedCandles []*ohlc.OHLC,
	openPositions []broker.Position,
) ([]broker.Order, []broker.Order, []broker.Position) {
	previousCandle := closedCandles[len(closedCandles)-2]

	var toClosePositions []broker.Position

	for i := range openPositions {
		openPosition := openPositions[i]
		if openPosition.Age(closedCandle.End) > maxAgeOpenPosition &&
			openPosition.PerformanceAbsolute(closedCandle.Close, closedCandle.Close) > 0 {
			toClosePositions = append(toClosePositions, openPosition)

			continue
		}

		switch openPosition.BuyDirection {
		case broker.BuyDirectionLong:
			if closedCandle.Close > previousCandle.High {
				toClosePositions = append(toClosePositions, openPosition)
			}
		case broker.BuyDirectionShort:
			if closedCandle.Close < previousCandle.Low {
				toClosePositions = append(toClosePositions, openPosition)
			}
		}
	}

	return nil, nil, toClosePositions
}

func (d *RSIADX) getADX() (float64, error) {
	adxValueMap, err := d.adx.Value()
	if err != nil {
		slog.Warn("Cannot get value from indicator", "error", err)

		return 0, fmt.Errorf("adx value: %w", err)
	}

	adxValue := adxValueMap[indicatoradx.Value]

	return adxValue, nil
}

func (d *RSIADX) getRSI() (float64, error) {
	rsiValueMap, err := d.rsi.Value()
	if err != nil {
		slog.Warn("Cannot get value from indicator", "error", err)

		return 0, fmt.Errorf("rsi value: %w", err)
	}

	rsiValue := rsiValueMap[indicatorrsi.Value]

	return rsiValue, nil
}

func (d *RSIADX) isRSILongSignal() bool {
	rsiValue, err := d.getRSI()

	_, rsiLowerThreshold := d.eo.RSI()

	return rsiValue <= rsiLowerThreshold && err == nil
}

func (d *RSIADX) isRSIShortSignal() bool {
	rsiValue, err := d.getRSI()

	rsiUpperThreshold, _ := d.eo.RSI()

	return rsiValue >= rsiUpperThreshold && err == nil
}

func (d *RSIADX) isStrongADXTrend() bool {
	adxValue, err := d.getADX()

	return adxValue >= adxThreshold && err == nil
}

func (d *RSIADX) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
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
