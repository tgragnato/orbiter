// Package eo implements Environment Overlays that adapt strategy setup towards market current momentum.
package eo

import (
	"sort"

	"github.com/tgragnato/orbiter/pkg/helper"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

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

// EnvironmentOverlay tracks market candles and price change percentiles
// to determine the current risk level for strategy adaptation.
type EnvironmentOverlay struct {
	candles             floatRing
	priceChangesPercent floatRing
}

// RiskLevel represents the current market risk classification.
type RiskLevel int

const minCandles = 50

// Risk level constants ordered from lowest to highest risk.
const (
	RLow RiskLevel = iota
	RModerate
	RHigh
	RExtreme
)

// DefaultRisk is the risk level used when insufficient data is available.
const DefaultRisk = RModerate

const (
	percentileQ1 = 25
	percentileQ2 = 50
	percentileQ3 = 75

	rsiUpperLow      = 80.0
	rsiLowerLow      = 20.0
	rsiUpperModerate = 85.0
	rsiLowerModerate = 15.0
	rsiUpperHigh     = 90.0
	rsiLowerHigh     = 10.0
	rsiUpperExtreme  = 95.0
	rsiLowerExtreme  = 5.0
)

// New creates a new EnvironmentOverlay with a default candle buffer.
func New() *EnvironmentOverlay {
	return &EnvironmentOverlay{
		candles:             newFloatRing(minCandles),
		priceChangesPercent: newFloatRing(minCandles),
	}
}

// AddCandle records a new OHLC candle into the overlay buffers.
func (eo *EnvironmentOverlay) AddCandle(candle *ohlc.OHLC) {
	eo.candles.push(candle.Close)

	perfPercent := candle.PerformanceFromOpenToHighAbsolute()
	eo.priceChangesPercent.push(perfPercent)
}

// RSI returns the upper and lower RSI thresholds based on the current risk level.
//
func (eo *EnvironmentOverlay) RSI() (float64, float64) {
	var riskLevel = eo.riskLevel()

	switch riskLevel {
	case RLow:
		return rsiUpperLow, rsiLowerLow
	case RModerate:
		return rsiUpperModerate, rsiLowerModerate
	case RHigh:
		return rsiUpperHigh, rsiLowerHigh
	case RExtreme:
		return rsiUpperExtreme, rsiLowerExtreme
	default:
		return rsiUpperModerate, rsiLowerModerate
	}
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
		priceChangeQ1 = helper.GetPercentile(sortedPriceChanges, percentileQ1)
		priceChangeQ2 = helper.GetPercentile(sortedPriceChanges, percentileQ2)
		priceChangeQ3 = helper.GetPercentile(sortedPriceChanges, percentileQ3)
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
