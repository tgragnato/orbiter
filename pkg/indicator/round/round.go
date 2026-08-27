// Copyright (c) 2019 Simon Klinkert
// Copyright (c) 2026 Tommaso Gragnato
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

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

	unitOne      = 1.0
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
		unit = unitOne
		multiplier = multiplierTen
	case rn.latestCandle.Close < thresholdHundred:
		unit = unitTen
		multiplier = multiplierHundred
	case rn.latestCandle.Close < thresholdThousand:
		unit = unitHundred
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
