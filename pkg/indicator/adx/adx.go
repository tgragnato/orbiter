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

package adx

import (
	"github.com/markcheno/go-talib"
	"github.com/tgragnato/orbiter/pkg/indicator"
	"github.com/tgragnato/orbiter/pkg/ohlc"
)

// Value is the map key for the ADX indicator result.
const Value = "ADX_VALUE"

const ringMultiplier = 2

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

// ADX computes the Average Directional Index indicator.
type ADX struct {
	closePrices floatRing
	highPrices  floatRing
	lowPrices   floatRing
	size        int
}

// New creates a new instance.
// size is usually 14.
func New(size int) *ADX {
	return &ADX{
		highPrices:  newFloatRing(size * ringMultiplier),
		lowPrices:   newFloatRing(size * ringMultiplier),
		closePrices: newFloatRing(size * ringMultiplier),
		size:        size,
	}
}

// Insert adds a new OHLC candle to the indicator.
func (v *ADX) Insert(candle *ohlc.OHLC) {
	if !candle.Closed() {
		return
	}

	v.closePrices.push(candle.Close)
	v.highPrices.push(candle.High)
	v.lowPrices.push(candle.Low)
}

// Value returns the current ADX indicator values.
func (v *ADX) Value() (map[string]float64, error) {
	closePrices := v.closePrices.values()
	highPrices := v.highPrices.values()

	lowPrices := v.lowPrices.values()

	minLen := v.size * ringMultiplier
	if len(closePrices) < minLen || len(highPrices) < minLen || len(lowPrices) < minLen {
		return nil, indicator.ErrNotEnoughData
	}

	var result = map[string]float64{}

	adx := talib.Adx(highPrices, lowPrices, closePrices, v.size)
	if len(adx) > 0 {
		result[Value] = adx[len(adx)-1]
	}

	return result, nil
}

// ValueResultKeys returns the map keys produced by Value.
func (v *ADX) ValueResultKeys() []string {
	return []string{Value}
}
