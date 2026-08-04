package eo

import (
	"sort"

	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Environment Overlays
// Adapt strategy setup towards market current momentum

// floatRing is a fixed-capacity FIFO queue that preserves insertion order.
// When full, the oldest element is dropped to make room for the new one.
type floatRing struct {
	buf []float64
	cap int
}

func newFloatRing(capacity int) floatRing {
	return floatRing{buf: make([]float64, 0, capacity), cap: capacity}
}

func (r *floatRing) push(v float64) {
	if len(r.buf) < r.cap {
		r.buf = append(r.buf, v)
	} else {
		copy(r.buf, r.buf[1:])
		r.buf[r.cap-1] = v
	}
}

func (r *floatRing) values() []float64 {
	return r.buf
}

type EnvironmentOverlay struct {
	candles             floatRing
	priceChangesPercent floatRing
}

type RiskLevel int

const (
	minCandles = 50

	RLow RiskLevel = iota
	RModerate
	RHigh
	RExtreme

	DefaultRisk = RModerate
)

func New() *EnvironmentOverlay {
	return &EnvironmentOverlay{
		candles:             newFloatRing(minCandles),
		priceChangesPercent: newFloatRing(minCandles),
	}
}

func (eo *EnvironmentOverlay) AddCandle(candle *ohlc.OHLC) {
	closePrice, _ := candle.Close.Float64()
	eo.candles.push(closePrice)

	perfPercent, _ := candle.PerformanceInPercentage().Float64()
	eo.priceChangesPercent.push(perfPercent)
}

func (eo *EnvironmentOverlay) riskLevel() RiskLevel {
	var prices = eo.candles.values()
	var priceChangesPercent = eo.priceChangesPercent.values()
	if len(prices) < minCandles || len(priceChangesPercent) < minCandles {
		return DefaultRisk
	}

	var lastPriceChange = priceChangesPercent[len(prices)-1]
	var sortedPriceChanges = priceChangesPercent
	sort.Float64s(sortedPriceChanges)

	var (
		priceChangeQ1 = helper.GetPercentile(sortedPriceChanges, 25)
		priceChangeQ2 = helper.GetPercentile(sortedPriceChanges, 50)
		priceChangeQ3 = helper.GetPercentile(sortedPriceChanges, 75)
	)

	switch {
	case lastPriceChange < priceChangeQ1:
		return RLow
	case lastPriceChange < priceChangeQ2:
		return RModerate
	case lastPriceChange < priceChangeQ3:
		return RHigh
	default:
		return RExtreme
	}
}

func (eo *EnvironmentOverlay) RSI() (upperThreshold, lowerThreshold float64) {
	var riskLevel = eo.riskLevel()

	switch riskLevel {
	case RLow:
		return 80, 20
	case RModerate:
		return 85, 15
	case RHigh:
		return 90, 10
	case RExtreme:
		return 95, 5
	default:
		return 85, 15
	}
}
