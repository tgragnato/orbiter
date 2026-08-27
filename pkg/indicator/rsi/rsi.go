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

package rsi

import (
	"github.com/markcheno/go-talib"
	"github.com/tgragnato/orbiter/pkg/indicator"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Value is the key used to store the RSI value in the result map.
const Value = "RSI_VALUE"

// ringSizeMultiplier controls how many periods of history are kept in the buffer.
const ringSizeMultiplier = 10

// floatRing is a fixed-capacity FIFO queue that preserves insertion order.
// When full, the oldest element is dropped to make room for the new one.
type floatRing struct {
	buf []float64
	cap int
}

func newFloatRing(capacity int) floatRing {
	return floatRing{buf: make([]float64, 0, capacity), cap: capacity}
}

func (r *floatRing) push(val float64) {
	if len(r.buf) < r.cap {
		r.buf = append(r.buf, val)
	} else {
		copy(r.buf, r.buf[1:])
		r.buf[r.cap-1] = val
	}
}

func (r *floatRing) values() []float64 {
	return r.buf
}

// RSI is a Relative Strength Index indicator.
type RSI struct {
	cb   floatRing
	size int
}

// New creates a new RSI indicator instance.
// size is usually 14.
func New(size int) *RSI {
	// The talib code seems to be doing a simple moving average for the initial n values,
	// and then do 1/n exponential smoothing thereafter. This is the standard Wilder's RSI.
	// I believe the calculations shold start at the beginning of the data and not using a sliding window which would
	// be problematic due to the simple moving average for the 1st n values.
	// So basically, in order for talib RSI to be calculated as 'accurate' as possible, the number of price points
	// I pass into the function should greatly exceed the number of price points needed to initialize the indicator
	// until you reach an n value where further increasing number of price points has a negligible effect on
	// the RSI value.
	// https://www.reddit.com/r/algotrading/comments/kmgmtt/cant_validate_rsi_indicator_values_from_talib_vs/
	return &RSI{
		cb:   newFloatRing(size * ringSizeMultiplier),
		size: size,
	}
}

// Insert adds a new OHLC candle to the RSI indicator.
func (v *RSI) Insert(o *ohlc.OHLC) {
	if !o.Closed() {
		return
	}

	v.cb.push(o.Close)
}

// Value computes the current RSI value.
func (v *RSI) Value() (map[string]float64, error) {
	result := map[string]float64{}

	closePrices := v.cb.values()

	if len(closePrices) < v.size+1 {
		return nil, indicator.ErrNotEnoughData
	}

	rsi := talib.Rsi(closePrices, v.size)
	if len(rsi) > 0 {
		result[Value] = rsi[len(rsi)-1]
	}

	return result, nil
}

// ValueResultKeys returns the list of keys produced by Value.
func (v *RSI) ValueResultKeys() []string {
	return []string{Value}
}
