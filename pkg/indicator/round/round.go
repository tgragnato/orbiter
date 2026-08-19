// Package round provides a round-number support/resistance indicator.
package round

import (
	"errors"
	"fmt"
	"math"

	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Sentinel errors for the round-number indicator.
var (
	ErrMissingPriceData = errors.New("price data is missing")
	ErrPriceTooHigh     = errors.New("not supported: price is too high")
)

// Price threshold and factor constants used in round-number calculations.
const (
	thresholdSubUnit  = 1.00
	thresholdTen      = 10.00
	thresholdHundred  = 100.00
	thresholdThousand = 1000.00
	thresholdTenK     = 10000.00

	subUnitCentFactor  = 100.0
	subUnitTenthFactor = 10.0

	unitTen      = 10.0
	unitHundred  = 100.0
	unitThousand = 1000.0

	multiplierTen      = 1.0
	multiplierHundred  = 0.1
	multiplierThousand = 0.01

	strongUpperMultiplier = 10.0
)

// Output map key constants for the Value method.
const (
	// LowerRoundNumberWeak is the nearest lower round number at the weak level.
	LowerRoundNumberWeak = "LowerRoundNumberWeak"
	// LowerRoundNumberStrong is the nearest lower round number at the strong level.
	LowerRoundNumberStrong = "LowerRoundNumberStrong"
	// UpperRoundNumberWeak is the nearest upper round number at the weak level.
	UpperRoundNumberWeak = "UpperRoundNumberWeak"
	// UpperRoundNumberStrong is the nearest upper round number at the strong level.
	UpperRoundNumberStrong = "UpperRoundNumberStrong"
)

// Number calculates round-number support and resistance levels for a price series.
type Number struct {
	latestCandle *ohlc.OHLC
}

// New constructs a new Number indicator.
func New() *Number {
	return &Number{
		latestCandle: nil,
	}
}

// Insert updates the indicator with the most recent OHLC candle.
func (rn *Number) Insert(o *ohlc.OHLC) {
	rn.latestCandle = o
}

// Value computes the four round-number levels for the current price.
func (rn *Number) Value() (map[string]float64, error) {
	if rn.latestCandle == nil {
		return nil, fmt.Errorf("%w", ErrMissingPriceData)
	}

	result := map[string]float64{}

	var unit float64

	var multiplier float64

	switch {
	case rn.latestCandle.Close < thresholdSubUnit:
		result[LowerRoundNumberWeak] = math.Floor(rn.latestCandle.Close*subUnitCentFactor) / subUnitCentFactor
		result[LowerRoundNumberStrong] = math.Floor(rn.latestCandle.Close*subUnitTenthFactor) / subUnitTenthFactor
		result[UpperRoundNumberWeak] = math.Ceil(rn.latestCandle.Close*subUnitCentFactor) / subUnitCentFactor
		result[UpperRoundNumberStrong] = math.Ceil(rn.latestCandle.Close*subUnitTenthFactor) / subUnitTenthFactor

		return result, nil
	case rn.latestCandle.Close < thresholdTen:
		unit = unitTen
		multiplier = multiplierTen
	case rn.latestCandle.Close < thresholdHundred:
		unit = unitHundred
		multiplier = multiplierHundred
	case rn.latestCandle.Close < thresholdThousand:
		unit = unitThousand
		multiplier = multiplierThousand
	case rn.latestCandle.Close < thresholdTenK:
		unit = unitThousand
		multiplier = multiplierThousand
	default:
		return nil, fmt.Errorf("%w", ErrPriceTooHigh)
	}

	result[LowerRoundNumberWeak] = math.Floor(rn.latestCandle.Close*multiplier) / multiplier
	result[LowerRoundNumberStrong] = unit
	result[UpperRoundNumberWeak] = math.Ceil(rn.latestCandle.Close*multiplier) / multiplier
	result[UpperRoundNumberStrong] = unit * strongUpperMultiplier

	return result, nil
}
