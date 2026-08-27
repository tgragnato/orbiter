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

package helper

import (
	"math"

	"github.com/tgragnato/orbiter/pkg/broker"
)

const percentageDivisor = 100.0

// CalcStopLossPriceByPercentage calculates the stop loss price given a price, percentage, and order direction.
func CalcStopLossPriceByPercentage(price, percentage float64, orderDirection broker.BuyDirection) float64 {
	percentFrom := price / percentageDivisor * percentage

	switch orderDirection {
	case broker.BuyDirectionLong:
		return price - percentFrom
	case broker.BuyDirectionShort:
		return price + percentFrom
	default:
		return 0
	}
}

// CalcTargetPriceByPercentage calculates the target price given a price, percentage, and order direction.
func CalcTargetPriceByPercentage(price, percentage float64, orderDirection broker.BuyDirection) float64 {
	percentFrom := price / percentageDivisor * percentage

	switch orderDirection {
	case broker.BuyDirectionLong:
		return price + percentFrom
	case broker.BuyDirectionShort:
		return price - percentFrom
	default:
		return 0
	}
}

// GetPercentile returns the value at the given percentile from a sorted slice of float64 values.
func GetPercentile(values []float64, percentile int) float64 {
	if len(values) == 0 {
		return 0.0
	}

	var pos = int(math.Round(float64(len(values)) / percentageDivisor * float64(percentile)))
	if pos < 1 || percentile == 0 {
		pos = 1
	}

	return values[pos-1]
}
