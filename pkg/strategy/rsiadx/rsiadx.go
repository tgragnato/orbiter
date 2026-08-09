package rsiadx

import (
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
	orderPricePrecision = 1
	adxThreshold        = 35
	adxCandles          = 10
	rsiCandles          = 2
	targetInPercent     = 5.0
	stopLossInPercent   = 2.5
	maxAgeOpenPosition  = time.Hour * 2
)

func New(instrument string, candleDuration time.Duration) *RSIADX {
	clog := slog.With("INSTRUMENT", instrument)

	return &RSIADX{
		clog:           clog,
		instrument:     instrument,
		rsi:            indicatorrsi.New(rsiCandles),
		adx:            indicatoradx.New(adxCandles),
		candleDuration: candleDuration,
		eo:             eo.New(),
	}
}

func (d *RSIADX) OnPosition(openPositions, _ []broker.Position) {
	d.openPositions = openPositions
}

func (d *RSIADX) OnOrder(openOrders []broker.Order) {
	d.openOrders = openOrders
}

func (d *RSIADX) OnTick(_ tick.Tick) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	return
}

func (d *RSIADX) GetCandleDuration() time.Duration {
	return d.candleDuration
}

func (d *RSIADX) OnWarmUpCandle(closedCandle *ohlc.OHLC) {
	d.rsi.Insert(closedCandle)
	d.adx.Insert(closedCandle)
	d.eo.AddCandle(closedCandle)
}

func (d *RSIADX) GetWarmUpCandleAmount() uint {
	return adxCandles * 2
}

func (d *RSIADX) OnCandle(closedCandles []*ohlc.OHLC) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	closedCandle := closedCandles[len(closedCandles)-1]

	d.rsi.Insert(closedCandle)
	d.adx.Insert(closedCandle)
	d.eo.AddCandle(closedCandle)

	if len(d.openPositions) > 0 {
		return d.checkOpenPositions(closedCandle, closedCandles, d.openPositions)
	}

	if !d.isStrongADXTrend() {
		return
	}

	if d.isRSIShortSignal() {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionShort, 1.00)
		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	} else if d.isRSILongSignal() {
		toOpenNew := d.prepareOrder(closedCandle, broker.BuyDirectionLong, 1.00)
		return []broker.Order{toOpenNew}, []broker.Order{}, []broker.Position{}
	}
	return
}

func (d *RSIADX) checkOpenPositions(closedCandle *ohlc.OHLC, closedCandles []*ohlc.OHLC, openPositions []broker.Position) (toOpen, toClose []broker.Order, toClosePositions []broker.Position) {
	var previousCandle = closedCandles[len(closedCandles)-2]

	for i := range openPositions {
		openPosition := openPositions[i]
		if openPosition.Age(closedCandle.End) > maxAgeOpenPosition &&
			openPosition.PerformanceAbsolute(closedCandle.Close, closedCandle.Close) > 0 {
			toClosePositions = append(toClosePositions, openPosition)
			continue
		}

		switch openPosition.BuyDirection {
		case broker.BuyDirectionLong:
			if closedCandle.Close.GreaterThan(previousCandle.High) {
				toClosePositions = append(toClosePositions, openPosition)
			}
		case broker.BuyDirectionShort:
			if closedCandle.Close.LessThan(previousCandle.Low) {
				toClosePositions = append(toClosePositions, openPosition)
			}
		}
	}
	return
}

func (d *RSIADX) isRSILongSignal() bool {
	var rsiValue, err = d.getRSI()
	var _, rsiLowerThreshold = d.eo.RSI()
	return rsiValue <= rsiLowerThreshold && err == nil
}

func (d *RSIADX) isRSIShortSignal() bool {
	var rsiValue, err = d.getRSI()
	var rsiUpperThreshold, _ = d.eo.RSI()
	return rsiValue >= rsiUpperThreshold && err == nil
}

func (d *RSIADX) isStrongADXTrend() bool {
	var adxValue, err = d.getADX()
	return adxValue >= adxThreshold && err == nil
}

func (d *RSIADX) getRSI() (rsiValue float64, err error) {
	rsiValueMap, err := d.rsi.Value()
	if err != nil {
		slog.Warn("Cannot get value from indicator", "error", err)
		return 0, err
	}
	rsiValue = rsiValueMap[indicatorrsi.Value]
	return
}

func (d *RSIADX) getADX() (adxValue float64, err error) {
	adxValueMap, err := d.adx.Value()
	if err != nil {
		slog.Warn("Cannot get value from indicator", "error", err)
		return 0, err
	}
	adxValue = adxValueMap[indicatoradx.Value]
	return
}

func (d *RSIADX) prepareOrder(closedCandle *ohlc.OHLC, direction broker.BuyDirection, size float64) broker.Order {
	var (
		targetPrice   = helper.CalcTargetPriceByPercentage(closedCandle.Close, helper.FloatToDecimal(targetInPercent), direction).Round(orderPricePrecision)
		stopLossPrice = helper.CalcStopLossPriceByPercentage(closedCandle.Close, helper.FloatToDecimal(stopLossInPercent), direction).Round(orderPricePrecision)
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

func (d *RSIADX) Name() string {
	return strategy.NameRSIADX
}

func (d *RSIADX) String() string {
	return d.Name()
}

// Score returns a conviction in [-1.0, +1.0]. Without a strong ADX trend the
// score is zero. With ADX ≥ threshold the RSI conviction is scaled by the
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
	adxScale := (adxVal - adxThreshold) / (100.0 - adxThreshold)
	if adxScale > 1 {
		adxScale = 1
	}

	const mid = 50.0
	var rsiConviction float64
	switch {
	case rsiVal <= lower:
		rsiConviction = 1.0
	case rsiVal >= upper:
		rsiConviction = -1.0
	case rsiVal < mid:
		rsiConviction = (mid - rsiVal) / (mid - lower)
	default:
		rsiConviction = -(rsiVal - mid) / (upper - mid)
	}

	return rsiConviction * adxScale
}
