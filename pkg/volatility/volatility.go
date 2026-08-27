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

// Package volatility computes price volatility metrics using a circular buffer of OHLC samples.
package volatility

import (
	"fmt"

	"github.com/tgragnato/orbiter/pkg/circularbuffer"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Volatility tracks price volatility using a circular buffer of historical OHLC data.
type Volatility struct {
	cb *circularbuffer.CircularBuffer
}

// New creates a new Volatility tracker with the given minimum and maximum buffer sizes.
func New(minSize, maxSize int) *Volatility {
	return &Volatility{
		cb: circularbuffer.New(minSize, maxSize),
	}
}

// AddOHLC records the volatility of a closed OHLC bar into the circular buffer.
func (vol *Volatility) AddOHLC(bar *ohlc.OHLC) {
	if !bar.Closed() {
		return
	}

	vol.cb.Insert(bar.VolatilityInPercentage())
}

// MedianVolatilityInPercentage returns the median volatility across all buffered samples.
func (vol *Volatility) MedianVolatilityInPercentage() (float64, error) {
	median, err := vol.cb.Median()
	if err != nil {
		return 0, fmt.Errorf("median volatility: %w", err)
	}

	return median, nil
}

// VolatilityInPercentageQuantile returns the volatility at the given quantile (0–1).
func (vol *Volatility) VolatilityInPercentageQuantile(quantile float64) (float64, error) {
	result, err := vol.cb.Quantile(quantile)
	if err != nil {
		return 0, fmt.Errorf("volatility quantile: %w", err)
	}

	return result, nil
}
